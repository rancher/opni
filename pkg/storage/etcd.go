package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/kralicky/opni-gateway/pkg/tokens"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var defaultEtcdTimeout = 5 * time.Second

type EtcdTokenStore struct {
	EtcdTokenStoreOptions
	client *clientv3.Client
}

type EtcdTokenStoreOptions struct {
	clientConfig clientv3.Config
}

type EtcdTokenStoreOption func(*EtcdTokenStoreOptions)

func (o *EtcdTokenStoreOptions) Apply(opts ...EtcdTokenStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithClientConfig(config clientv3.Config) EtcdTokenStoreOption {
	return func(o *EtcdTokenStoreOptions) {
		o.clientConfig = config
	}
}

func NewEtcdTokenStore(opts ...EtcdTokenStoreOption) (TokenStore, error) {
	options := &EtcdTokenStoreOptions{}
	options.Apply(opts...)
	cli, err := clientv3.New(options.clientConfig)
	if err != nil {
		return nil, err
	}
	ctx, ca := context.WithTimeout(context.Background(), defaultEtcdTimeout)
	defer ca()
	_, err = cli.Status(ctx, options.clientConfig.Endpoints[0])
	if err != nil {
		return nil, err
	}
	fmt.Printf("Connected to etcd at %s\n", options.clientConfig.Endpoints)
	return &EtcdTokenStore{
		client: cli,
	}, nil
}

func (v *EtcdTokenStore) CreateToken(ctx context.Context, ttl time.Duration) (*tokens.Token, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	lease, err := v.client.Grant(ctx, int64(ttl.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}
	token := tokens.NewToken()
	token.Metadata.LeaseID = int64(lease.ID)
	token.Metadata.TTL = lease.TTL
	_, err = v.client.Put(ctx, "/tokens/"+token.HexID(), token.EncodeJSON(),
		clientv3.WithLease(lease.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to create token %w", err)
	}
	return token, nil
}

func (v *EtcdTokenStore) DeleteToken(ctx context.Context, tokenID string) error {
	t, err := v.GetToken(ctx, tokenID)
	if err != nil {
		return err
	}
	// If the token has a lease, revoke it, which will delete the token.
	if t.Metadata.LeaseID != 0 {
		_, err := v.client.Revoke(context.Background(), clientv3.LeaseID(t.Metadata.LeaseID))
		if err != nil {
			return fmt.Errorf("failed to revoke lease %d: %w", t.Metadata.LeaseID, err)
		}
		return nil
	}
	// If the token doesn't have a lease, delete it directly.
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := v.client.Delete(ctx, "/tokens/"+tokenID)
	if err != nil {
		return fmt.Errorf("failed to delete token %s: %w", tokenID, err)
	}
	if resp.Deleted == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (v *EtcdTokenStore) TokenExists(ctx context.Context, tokenID string) (bool, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := v.client.Get(ctx, "/tokens/"+tokenID)
	if err != nil {
		return false, fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}
	return len(resp.Kvs) > 0, nil
}

func (v *EtcdTokenStore) GetToken(ctx context.Context, tokenID string) (*tokens.Token, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := v.client.Get(ctx, "/tokens/"+tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("failed to get token %s: %w", tokenID, ErrTokenNotFound)
	}
	kv := resp.Kvs[0]
	token, err := tokens.DecodeJSONToken(kv.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to decode token %s: %w", tokenID, err)
	}
	if err := v.addLeaseMetadata(ctx, token, kv.Lease); err != nil {
		return nil, err
	}
	return token, nil
}

func (v *EtcdTokenStore) ListTokens(ctx context.Context) ([]*tokens.Token, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := v.client.Get(ctx, "/tokens/", clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	items := make([]*tokens.Token, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		token, err := tokens.DecodeJSONToken(kv.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to decode token %s: %w", string(kv.Value), err)
		}
		if err := v.addLeaseMetadata(ctx, token, kv.Lease); err != nil {
			return nil, err
		}
		items[i] = token
	}
	return items, nil
}

func (v *EtcdTokenStore) addLeaseMetadata(
	ctx context.Context,
	token *tokens.Token,
	lease int64,
) error {
	if lease != 0 {
		token.Metadata.LeaseID = lease
		// lookup lease
		leaseResp, err := v.client.TimeToLive(ctx, clientv3.LeaseID(lease))
		if err != nil {
			fmt.Errorf("failed to get lease %d: %w", lease, err)
		}
		token.Metadata.TTL = leaseResp.TTL
	}
	return nil
}
