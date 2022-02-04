package etcd

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/tokens"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

func (e *EtcdStore) CreateToken(ctx context.Context, ttl time.Duration) (*tokens.Token, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	lease, err := e.client.Grant(ctx, int64(ttl.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}
	token := tokens.NewToken()
	token.Metadata.LeaseID = int64(lease.ID)
	token.Metadata.TTL = lease.TTL
	_, err = e.client.Put(ctx, path.Join(e.namespace, tokensKey, token.HexID()), string(token.EncodeJSON()),
		clientv3.WithLease(lease.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to create token %w", err)
	}
	return token, nil
}

func (e *EtcdStore) DeleteToken(ctx context.Context, ref *core.Reference) error {
	if err := ref.CheckValidID(); err != nil {
		return err
	}
	t, err := e.GetToken(ctx, ref)
	if err != nil {
		return err
	}
	// If the token has a lease, revoke it, which will delete the token.
	if t.Metadata.LeaseID != 0 {
		_, err := e.client.Revoke(context.Background(), clientv3.LeaseID(t.Metadata.LeaseID))
		if err != nil {
			return fmt.Errorf("failed to revoke lease: %w", err)
		}
		return nil
	}
	// If the token doesn't have a lease, delete it directly.
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Delete(ctx, path.Join(e.namespace, tokensKey, ref.Id))
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}
	if resp.Deleted == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (e *EtcdStore) TokenExists(ctx context.Context, ref *core.Reference) (bool, error) {
	if err := ref.CheckValidID(); err != nil {
		return false, err
	}
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, path.Join(e.namespace, tokensKey, ref.Id))
	if err != nil {
		return false, fmt.Errorf("failed to get token: %w", err)
	}
	return len(resp.Kvs) > 0, nil
}

func (e *EtcdStore) GetToken(ctx context.Context, ref *core.Reference) (*tokens.Token, error) {
	if err := ref.CheckValidID(); err != nil {
		return nil, err
	}
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, path.Join(e.namespace, tokensKey, ref.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("failed to get token: %w", storage.ErrNotFound)
	}
	kv := resp.Kvs[0]
	token, err := tokens.ParseJSON(kv.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to decode token: %w", err)
	}
	if err := e.addLeaseMetadata(ctx, token, kv.Lease); err != nil {
		return nil, err
	}
	return token, nil
}

func (e *EtcdStore) ListTokens(ctx context.Context) ([]*tokens.Token, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, path.Join(e.namespace, tokensKey), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	items := make([]*tokens.Token, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		token, err := tokens.ParseJSON(kv.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to decode token: %w", err)
		}
		if err := e.addLeaseMetadata(ctx, token, kv.Lease); err != nil {
			return nil, err
		}
		items[i] = token
	}
	return items, nil
}

func (e *EtcdStore) addLeaseMetadata(
	ctx context.Context,
	token *tokens.Token,
	lease int64,
) error {
	if lease != 0 {
		token.Metadata.LeaseID = lease
		// lookup lease
		leaseResp, err := e.client.TimeToLive(ctx, clientv3.LeaseID(lease))
		if err != nil {
			e.logger.With(
				zap.Error(err),
				zap.Int64("lease", lease),
			).Error("failed to get lease")
		} else {
			token.Metadata.TTL = leaseResp.TTL
		}
	}
	return nil
}
