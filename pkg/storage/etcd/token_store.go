package etcd

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/tokens"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/encoding/protojson"
)

func (e *EtcdStore) CreateToken(ctx context.Context, ttl time.Duration, labels map[string]string) (*core.BootstrapToken, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	lease, err := e.client.Grant(ctx, int64(ttl.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}
	token := tokens.NewToken().ToBootstrapToken()
	token.Metadata = &core.BootstrapTokenMetadata{
		LeaseID:    int64(lease.ID),
		UsageCount: 0,
		Labels:     labels,
	}
	data, err := protojson.Marshal(token)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token: %w", err)
	}

	_, err = e.client.Put(ctx, path.Join(e.namespace, tokensKey, token.TokenID), string(data),
		clientv3.WithLease(lease.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to create token %w", err)
	}
	return token, nil
}

func (e *EtcdStore) DeleteToken(ctx context.Context, ref *core.Reference) error {
	t, err := e.GetToken(ctx, ref)
	if err != nil {
		return err
	}
	if t.Metadata.LeaseID != 0 {
		defer func(id int64) {
			_, err := e.client.Revoke(context.Background(), clientv3.LeaseID(id))
			if err != nil {
				e.logger.Warnf("failed to revoke lease: %v", err)
			}
		}(t.Metadata.LeaseID)
	}
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

func (e *EtcdStore) GetToken(ctx context.Context, ref *core.Reference) (*core.BootstrapToken, error) {
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
	token := &core.BootstrapToken{}
	if err := protojson.Unmarshal(kv.Value, token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}
	if err := e.addLeaseMetadata(ctx, token, kv.Lease); err != nil {
		return nil, err
	}
	return token, nil
}

func (e *EtcdStore) ListTokens(ctx context.Context) ([]*core.BootstrapToken, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, path.Join(e.namespace, tokensKey), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	items := make([]*core.BootstrapToken, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		token := &core.BootstrapToken{}
		if err := protojson.Unmarshal(kv.Value, token); err != nil {
			return nil, fmt.Errorf("failed to unmarshal token: %w", err)
		}
		if err := e.addLeaseMetadata(ctx, token, kv.Lease); err != nil {
			return nil, err
		}
		items[i] = token
	}
	return items, nil
}

var retryErr = errors.New("failed to increment token usage count: the token has been modified, retrying")

func (e *EtcdStore) IncrementUsageCount(ctx context.Context, ref *core.Reference) error {
	key := path.Join(e.namespace, tokensKey, ref.Id)
	for {
		err := e.tryIncrementUsageCount(ctx, key)
		if err != nil {
			if errors.Is(err, retryErr) {
				e.logger.Warn(err)
				continue
			}
			return err
		}
		return nil
	}
}

func (e *EtcdStore) tryIncrementUsageCount(ctx context.Context, key string) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	txn := e.client.Txn(ctx)
	resp, err := e.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return fmt.Errorf("failed to get token: %w", storage.ErrNotFound)
	}
	kv := resp.Kvs[0]
	token := &core.BootstrapToken{}
	if err := protojson.Unmarshal(kv.Value, token); err != nil {
		return fmt.Errorf("failed to unmarshal token: %w", err)
	}

	token.Metadata.UsageCount++
	data, err := protojson.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	txnResp, err := txn.If(clientv3.Compare(clientv3.Version(key), "=", kv.Version)).
		Then(clientv3.OpPut(key, string(data), clientv3.WithIgnoreLease())).
		Commit()
	if err != nil {
		return fmt.Errorf("failed to increment token usage count: %w", err)
	}
	if !txnResp.Succeeded {
		return retryErr
	}
	return nil
}

func (e *EtcdStore) addLeaseMetadata(
	ctx context.Context,
	token *core.BootstrapToken,
	lease int64,
) error {
	if lease != 0 {
		token.Metadata.LeaseID = lease
		// lookup lease
		leaseResp, err := e.client.TimeToLive(ctx, clientv3.LeaseID(lease))
		if err != nil {
			return fmt.Errorf("failed to get lease: %w", err)
		} else {
			token.Metadata.Ttl = leaseResp.TTL
		}
	}
	return nil
}
