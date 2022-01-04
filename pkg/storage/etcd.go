package storage

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/kralicky/opni-gateway/pkg/keyring"
	"github.com/kralicky/opni-gateway/pkg/tokens"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var defaultEtcdTimeout = 5 * time.Second

// EtcdStore implements TokenStore and TenantStore.
type EtcdStore struct {
	EtcdStoreOptions
	client *clientv3.Client
}

var _ TokenStore = (*EtcdStore)(nil)
var _ TenantStore = (*EtcdStore)(nil)

type EtcdStoreOptions struct {
	clientConfig clientv3.Config
}

type EtcdStoreOption func(*EtcdStoreOptions)

func (o *EtcdStoreOptions) Apply(opts ...EtcdStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithClientConfig(config clientv3.Config) EtcdStoreOption {
	return func(o *EtcdStoreOptions) {
		o.clientConfig = config
	}
}

func NewEtcdStore(opts ...EtcdStoreOption) *EtcdStore {
	options := &EtcdStoreOptions{}
	options.Apply(opts...)
	cli, err := clientv3.New(options.clientConfig)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to create etcd client: %w", err))
	}
	ctx, ca := context.WithTimeout(context.Background(), defaultEtcdTimeout)
	defer ca()
	_, err = cli.Status(ctx, options.clientConfig.Endpoints[0])
	if err != nil {
		log.Fatal(fmt.Errorf("failed to connect to etcd: %w", err))
	}
	fmt.Printf("Connected to etcd at %s\n", options.clientConfig.Endpoints)
	return &EtcdStore{
		client: cli,
	}
}

func (v *EtcdStore) CreateToken(ctx context.Context, ttl time.Duration) (*tokens.Token, error) {
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

func (v *EtcdStore) DeleteToken(ctx context.Context, tokenID string) error {
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
		return ErrNotFound
	}
	return nil
}

func (v *EtcdStore) TokenExists(ctx context.Context, tokenID string) (bool, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := v.client.Get(ctx, "/tokens/"+tokenID)
	if err != nil {
		return false, fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}
	return len(resp.Kvs) > 0, nil
}

func (v *EtcdStore) GetToken(ctx context.Context, tokenID string) (*tokens.Token, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := v.client.Get(ctx, "/tokens/"+tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("failed to get token %s: %w", tokenID, ErrNotFound)
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

func (v *EtcdStore) ListTokens(ctx context.Context) ([]*tokens.Token, error) {
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

func (v *EtcdStore) addLeaseMetadata(
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

func (v *EtcdStore) CreateTenant(ctx context.Context, tenantID string) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	_, err := v.client.Put(ctx, "/tenants/"+tenantID, "")
	if err != nil {
		return fmt.Errorf("failed to create tenant %s: %w", tenantID, err)
	}
	return nil
}

func (v *EtcdStore) DeleteTenant(ctx context.Context, tenantID string) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	_, err := v.client.Delete(ctx, "/tenants/"+tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete tenant %s: %w", tenantID, err)
	}
	return nil
}

func (v *EtcdStore) TenantExists(ctx context.Context, tenantID string) (bool, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := v.client.Get(ctx, "/tenants/"+tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to get tenant %s: %w", tenantID, err)
	}
	return len(resp.Kvs) > 0, nil
}

func (v *EtcdStore) ListTenants(ctx context.Context) ([]string, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := v.client.Get(ctx, "/tenants/", clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	items := make([]string, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		items[i] = string(kv.Key)
	}
	return items, nil
}

type tenantKeyringStore struct {
	client   *clientv3.Client
	tenantID string
}

func (v *EtcdStore) KeyringStore(ctx context.Context, tenantID string) (KeyringStore, error) {
	return &tenantKeyringStore{
		client:   v.client,
		tenantID: tenantID,
	}, nil
}

func (ks *tenantKeyringStore) Put(ctx context.Context, keyring keyring.Keyring) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	k, err := keyring.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal keyring: %w", err)
	}
	_, err = ks.client.Put(ctx, fmt.Sprintf("/tenants/%s/keyring", ks.tenantID), string(k))
	if err != nil {
		return fmt.Errorf("failed to put keyring: %w", err)
	}
	return nil
}

func (ks *tenantKeyringStore) Get(ctx context.Context) (keyring.Keyring, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := ks.client.Get(ctx, fmt.Sprintf("/tenants/%s/keyring", ks.tenantID))
	if err != nil {
		return nil, fmt.Errorf("failed to get keyring: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, ErrNotFound
	}
	k, err := keyring.Unmarshal(resp.Kvs[0].Value)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal keyring: %w", err)
	}
	return k, nil
}
