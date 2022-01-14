package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kralicky/opni-monitoring/pkg/keyring"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/rbac"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

var defaultEtcdTimeout = 5 * time.Second

// EtcdStore implements TokenStore and TenantStore.
type EtcdStore struct {
	EtcdStoreOptions
	logger *zap.SugaredLogger
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
	lg := logger.New().Named("etcd")
	cli, err := clientv3.New(options.clientConfig)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to create etcd client")
	}
	ctx, ca := context.WithTimeout(context.Background(), defaultEtcdTimeout)
	defer ca()
	_, err = cli.Status(ctx, options.clientConfig.Endpoints[0])
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to connect to etcd")
	}
	lg.With(
		"endpoints", options.clientConfig.Endpoints,
	).Info("Connected to etcd")
	return &EtcdStore{
		client: cli,
	}
}

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
	_, err = e.client.Put(ctx, "/tokens/"+token.HexID(), string(token.EncodeJSON()),
		clientv3.WithLease(lease.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to create token %w", err)
	}
	return token, nil
}

func (e *EtcdStore) DeleteToken(ctx context.Context, tokenID string) error {
	t, err := e.GetToken(ctx, tokenID)
	if err != nil {
		return err
	}
	// If the token has a lease, revoke it, which will delete the token.
	if t.Metadata.LeaseID != 0 {
		_, err := e.client.Revoke(context.Background(), clientv3.LeaseID(t.Metadata.LeaseID))
		if err != nil {
			return fmt.Errorf("failed to revoke lease %d: %w", t.Metadata.LeaseID, err)
		}
		return nil
	}
	// If the token doesn't have a lease, delete it directly.
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Delete(ctx, "/tokens/"+tokenID)
	if err != nil {
		return fmt.Errorf("failed to delete token %s: %w", tokenID, err)
	}
	if resp.Deleted == 0 {
		return ErrNotFound
	}
	return nil
}

func (e *EtcdStore) TokenExists(ctx context.Context, tokenID string) (bool, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, "/tokens/"+tokenID)
	if err != nil {
		return false, fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}
	return len(resp.Kvs) > 0, nil
}

func (e *EtcdStore) GetToken(ctx context.Context, tokenID string) (*tokens.Token, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, "/tokens/"+tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token %s: %w", tokenID, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("failed to get token %s: %w", tokenID, ErrNotFound)
	}
	kv := resp.Kvs[0]
	token, err := tokens.ParseJSON(kv.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to decode token %s: %w", tokenID, err)
	}
	if err := e.addLeaseMetadata(ctx, token, kv.Lease); err != nil {
		return nil, err
	}
	return token, nil
}

func (e *EtcdStore) ListTokens(ctx context.Context) ([]*tokens.Token, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, "/tokens/", clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	items := make([]*tokens.Token, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		token, err := tokens.ParseJSON(kv.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to decode token %s: %w", string(kv.Value), err)
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

func (e *EtcdStore) CreateTenant(ctx context.Context, tenantID string) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	_, err := e.client.Put(ctx, "/tenants/"+tenantID, "")
	if err != nil {
		return fmt.Errorf("failed to create tenant %s: %w", tenantID, err)
	}
	return nil
}

func (e *EtcdStore) DeleteTenant(ctx context.Context, tenantID string) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	_, err := e.client.Delete(ctx, "/tenants/"+tenantID, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to delete tenant %s: %w", tenantID, err)
	}
	return nil
}

func (e *EtcdStore) TenantExists(ctx context.Context, tenantID string) (bool, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, "/tenants/"+tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to get tenant %s: %w", tenantID, err)
	}
	return len(resp.Kvs) > 0, nil
}

func (e *EtcdStore) ListTenants(ctx context.Context) ([]string, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, "/tenants/", clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	// Keys will be of the form /tenants/<tenantID>[/keyring]
	ids := map[string]struct{}{}
	for _, kv := range resp.Kvs {
		parts := strings.Split(string(kv.Key), "/") // {"", "tenants", <tenantID> [, ...]}
		if len(parts) < 3 {
			return nil, fmt.Errorf("unexpected key %s", kv.Key)
		}
		ids[parts[2]] = struct{}{}
	}
	sortedIds := make([]string, 0, len(ids))
	for id := range ids {
		sortedIds = append(sortedIds, id)
	}
	sort.Strings(sortedIds)
	return sortedIds, nil
}

func (e *EtcdStore) CreateRole(ctx context.Context, roleName string, tenantIDs []string) (rbac.Role, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	role := rbac.Role{
		Name:      roleName,
		TenantIDs: tenantIDs,
	}
	data, err := json.Marshal(role)
	if err != nil {
		return rbac.Role{}, fmt.Errorf("failed to marshal role %s: %w", roleName, err)
	}
	_, err = e.client.Put(ctx, "/roles/"+roleName, string(data))
	if err != nil {
		return rbac.Role{}, fmt.Errorf("failed to create role %s: %w", roleName, err)
	}
	return role, nil
}

func (e *EtcdStore) DeleteRole(ctx context.Context, roleName string) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	_, err := e.client.Delete(ctx, "/roles/"+roleName)
	if err != nil {
		return fmt.Errorf("failed to delete role %s: %w", roleName, err)
	}
	return nil
}

func (e *EtcdStore) GetRole(ctx context.Context, roleName string) (rbac.Role, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, "/roles/"+roleName)
	if err != nil {
		return rbac.Role{}, fmt.Errorf("failed to get role %s: %w", roleName, err)
	}
	if len(resp.Kvs) == 0 {
		return rbac.Role{}, fmt.Errorf("failed to get role %s: %w", roleName, ErrNotFound)
	}
	var role rbac.Role
	if err := json.Unmarshal(resp.Kvs[0].Value, &role); err != nil {
		return rbac.Role{}, fmt.Errorf("failed to unmarshal role %s: %w", roleName, err)
	}
	return role, nil
}

func (e *EtcdStore) CreateRoleBinding(ctx context.Context, roleBindingName string, roleName string, userID string) (rbac.RoleBinding, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	roleBinding := rbac.RoleBinding{
		Name:     roleBindingName,
		RoleName: roleName,
		UserID:   userID,
	}
	data, err := json.Marshal(roleBinding)
	if err != nil {
		return rbac.RoleBinding{}, fmt.Errorf("failed to marshal role binding %s: %w", roleBindingName, err)
	}
	_, err = e.client.Put(ctx, "/role_bindings/"+roleBindingName, string(data))
	if err != nil {
		return rbac.RoleBinding{}, fmt.Errorf("failed to create role binding %s: %w", roleBindingName, err)
	}
	return roleBinding, nil
}

func (e *EtcdStore) DeleteRoleBinding(ctx context.Context, roleBindingName string) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	_, err := e.client.Delete(ctx, "/role_bindings/"+roleBindingName)
	if err != nil {
		return fmt.Errorf("failed to delete role binding %s: %w", roleBindingName, err)
	}
	return nil
}

func (e *EtcdStore) GetRoleBinding(ctx context.Context, roleBindingName string) (rbac.RoleBinding, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, "/role_bindings/"+roleBindingName)
	if err != nil {
		return rbac.RoleBinding{}, fmt.Errorf("failed to get role binding %s: %w", roleBindingName, err)
	}
	if len(resp.Kvs) == 0 {
		return rbac.RoleBinding{}, fmt.Errorf("failed to get role binding %s: %w", roleBindingName, ErrNotFound)
	}
	var roleBinding rbac.RoleBinding
	if err := json.Unmarshal(resp.Kvs[0].Value, &roleBinding); err != nil {
		return rbac.RoleBinding{}, fmt.Errorf("failed to unmarshal role binding %s: %w", roleBindingName, err)
	}
	return roleBinding, nil
}

func (e *EtcdStore) ListRoles(ctx context.Context) ([]rbac.Role, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, "/roles/", clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	items := make([]rbac.Role, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		var role rbac.Role
		if err := json.Unmarshal(kv.Value, &role); err != nil {
			return nil, fmt.Errorf("failed to decode role %s: %w", string(kv.Value), err)
		}
		items[i] = role
	}
	return items, nil
}

func (e *EtcdStore) ListRoleBindings(ctx context.Context) ([]rbac.RoleBinding, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, "/role_bindings/", clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list role bindings: %w", err)
	}
	items := make([]rbac.RoleBinding, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		var roleBinding rbac.RoleBinding
		if err := json.Unmarshal(kv.Value, &roleBinding); err != nil {
			return nil, fmt.Errorf("failed to decode role binding %s: %w", string(kv.Value), err)
		}
		items[i] = roleBinding
	}
	return items, nil
}

type tenantKeyringStore struct {
	client   *clientv3.Client
	tenantID string
}

func (e *EtcdStore) KeyringStore(ctx context.Context, tenantID string) (KeyringStore, error) {
	return &tenantKeyringStore{
		client:   e.client,
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
