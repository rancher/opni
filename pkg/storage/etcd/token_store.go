package etcd

import (
	"context"
	"fmt"
	"path"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/lestrrat-go/backoff/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/tokens"
)

func (e *EtcdStore) CreateToken(ctx context.Context, ttl time.Duration, opts ...storage.TokenCreateOption) (*corev1.BootstrapToken, error) {
	options := storage.NewTokenCreateOptions()
	options.Apply(opts...)

	lease, err := e.Client.Grant(ctx, int64(ttl.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}
	token := tokens.NewToken().ToBootstrapToken()
	token.Metadata = &corev1.BootstrapTokenMetadata{
		LeaseID:      int64(lease.ID),
		UsageCount:   0,
		Labels:       options.Labels,
		Capabilities: options.Capabilities,
		MaxUsages:    options.MaxUsages,
	}
	data, err := protojson.Marshal(token)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token: %w", err)
	}
	_, err = e.Client.Put(ctx, path.Join(e.Prefix, tokensKey, token.TokenID), string(data),
		clientv3.WithLease(lease.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to create token %w", err)
	}
	token.Metadata.Ttl = int64(ttl.Seconds())
	token.SetResourceVersion("1")
	return token, nil
}

func (e *EtcdStore) DeleteToken(ctx context.Context, ref *corev1.Reference) error {
	t, err := e.GetToken(ctx, ref)
	if err != nil {
		return err
	}
	if t.Metadata.LeaseID != 0 {
		defer func(id int64) {
			_, err := e.Client.Revoke(context.Background(), clientv3.LeaseID(id))
			if err != nil {
				e.Logger.Warn(fmt.Sprintf("failed to revoke lease: %v", err))
			}
		}(t.Metadata.LeaseID)
	}
	resp, err := e.Client.Delete(ctx, path.Join(e.Prefix, tokensKey, ref.Id))
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}
	if resp.Deleted == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (e *EtcdStore) GetToken(ctx context.Context, ref *corev1.Reference) (*corev1.BootstrapToken, error) {
	t, _, err := e.getToken(ctx, ref)
	if err != nil {
		return nil, err
	}

	// If the token has reached its max usage, delete it
	if t.MaxUsageReached() {
		go e.garbageCollectToken(t)
		return nil, storage.ErrNotFound
	}
	return t, err
}

func (e *EtcdStore) getToken(ctx context.Context, ref *corev1.Reference) (*corev1.BootstrapToken, int64, error) {
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, tokensKey, ref.Id))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get token: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, 0, storage.ErrNotFound
	}
	kv := resp.Kvs[0]
	token := &corev1.BootstrapToken{}
	if err := protojson.Unmarshal(kv.Value, token); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal token: %w", err)
	}
	if err := e.addLeaseMetadata(ctx, token, kv.Lease); err != nil {
		return nil, 0, err
	}
	return token, kv.Version, nil
}

func (e *EtcdStore) ListTokens(ctx context.Context) ([]*corev1.BootstrapToken, error) {
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, tokensKey), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	items := make([]*corev1.BootstrapToken, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		token := &corev1.BootstrapToken{}
		if err := protojson.Unmarshal(kv.Value, token); err != nil {
			return nil, fmt.Errorf("failed to unmarshal token: %w", err)
		}
		if token.MaxUsageReached() {
			go e.garbageCollectToken(token)
			continue
		}
		if err := e.addLeaseMetadata(ctx, token, kv.Lease); err != nil {
			return nil, err
		}
		items = append(items, token)
	}
	return items, nil
}

func (e *EtcdStore) UpdateToken(ctx context.Context, ref *corev1.Reference, mutator storage.MutatorFunc[*corev1.BootstrapToken]) (*corev1.BootstrapToken, error) {
	var retToken *corev1.BootstrapToken
	retryFunc := func() error {
		txn := e.Client.Txn(ctx)
		key := path.Join(e.Prefix, tokensKey, ref.Id)
		token, version, err := e.getToken(ctx, ref)
		if err != nil {
			return err
		}
		mutator(token)
		data, err := protojson.Marshal(token)
		if err != nil {
			return fmt.Errorf("failed to marshal token: %w", err)
		}
		txnResp, err := txn.If(clientv3.Compare(clientv3.Version(key), "=", version)).
			Then(clientv3.OpPut(key, string(data), clientv3.WithIgnoreLease())).
			Commit()
		if err != nil {
			e.Logger.With(
				logger.Err(err),
			).Error("error updating token")
			return err
		}
		if !txnResp.Succeeded {
			return errRetry
		}
		retToken = token
		return nil
	}
	c := defaultBackoff.Start(ctx)
	var err error
	for backoff.Continue(c) {
		err = retryFunc()
		if isRetryErr(err) {
			continue
		}
		break
	}
	if err != nil {
		return nil, err
	}
	return retToken, nil
}

func (e *EtcdStore) addLeaseMetadata(
	ctx context.Context,
	token *corev1.BootstrapToken,
	lease int64,
) error {
	if lease != 0 {
		token.Metadata.LeaseID = lease
		// lookup lease
		leaseResp, err := e.Client.TimeToLive(ctx, clientv3.LeaseID(lease))
		if err != nil {
			return fmt.Errorf("failed to get lease: %w", err)
		}
		token.Metadata.Ttl = leaseResp.TTL
	}
	return nil
}

// garbageCollectToken performs a best-effort deletion of an expired token.
func (e *EtcdStore) garbageCollectToken(token *corev1.BootstrapToken) {
	e.Logger.With(
		"token", token.GetTokenID(),
	).Debug("garbage-collecting expired token")

	if token.Metadata.LeaseID != 0 {
		defer func(id int64) {
			_, err := e.Client.Revoke(context.Background(), clientv3.LeaseID(id))
			if err != nil {
				e.Logger.Warn(fmt.Sprintf("failed to revoke lease: %v", err))
			}
		}(token.Metadata.LeaseID)
	}

	_, err := e.Client.Delete(context.Background(), path.Join(e.Prefix, tokensKey, token.TokenID))
	if err != nil {
		e.Logger.With(
			"token", token.TokenID,
			"error", err,
		).Warn("failed to garbage-collect expired token")
	}
}
