package etcd

import (
	"context"
	"crypto/tls"
	"fmt"
	"path"
	"time"

	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

var defaultEtcdTimeout = 5 * time.Second

const (
	tokensKey      = "tokens"
	clusterKey     = "clusters"
	keyringKey     = "keyrings"
	roleKey        = "roles"
	roleBindingKey = "rolebindings"
)

// EtcdStore implements TokenStore and TenantStore.
type EtcdStore struct {
	EtcdStoreOptions
	logger *zap.SugaredLogger
	client *clientv3.Client
}

var _ storage.TokenStore = (*EtcdStore)(nil)
var _ storage.ClusterStore = (*EtcdStore)(nil)
var _ storage.RBACStore = (*EtcdStore)(nil)

type EtcdStoreOptions struct {
	namespace string
}

type EtcdStoreOption func(*EtcdStoreOptions)

func (o *EtcdStoreOptions) Apply(opts ...EtcdStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithNamespace(namespace string) EtcdStoreOption {
	return func(o *EtcdStoreOptions) {
		o.namespace = namespace
	}
}

func NewEtcdStore(conf *v1beta1.EtcdStorageSpec, opts ...EtcdStoreOption) *EtcdStore {
	options := &EtcdStoreOptions{}
	options.Apply(opts...)
	lg := logger.New().Named("etcd")
	var tlsConfig *tls.Config
	if conf.Certs != nil {
		var err error
		tlsConfig, err = util.LoadClientMTLSConfig(conf.Certs)
		if err != nil {
			lg.Fatal("failed to load client TLS config", zap.Error(err))
		}
	}
	clientConfig := clientv3.Config{
		Endpoints: conf.Endpoints,
		TLS:       tlsConfig,
	}
	cli, err := clientv3.New(clientConfig)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to create etcd client")
	}
	lg.With(
		"endpoints", clientConfig.Endpoints,
	).Info("connecting to etcd")
	return &EtcdStore{
		client: cli,
	}
}

type clusterKeyringStore struct {
	client     *clientv3.Client
	clusterRef *core.Reference
	namespace  string
}

func (e *EtcdStore) KeyringStore(ctx context.Context, ref *core.Reference) (storage.KeyringStore, error) {
	if err := ref.CheckValidID(); err != nil {
		return nil, err
	}
	return &clusterKeyringStore{
		client:     e.client,
		clusterRef: ref,
		namespace:  e.namespace,
	}, nil
}

func (ks *clusterKeyringStore) Put(ctx context.Context, keyring keyring.Keyring) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	k, err := keyring.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal keyring: %w", err)
	}
	_, err = ks.client.Put(ctx, path.Join(ks.namespace, keyringKey, ks.clusterRef.Id), string(k))
	if err != nil {
		return fmt.Errorf("failed to put keyring: %w", err)
	}
	return nil
}

func (ks *clusterKeyringStore) Get(ctx context.Context) (keyring.Keyring, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := ks.client.Get(ctx, path.Join(ks.namespace, keyringKey, ks.clusterRef.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to get keyring: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, storage.ErrNotFound
	}
	k, err := keyring.Unmarshal(resp.Kvs[0].Value)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal keyring: %w", err)
	}
	return k, nil
}
