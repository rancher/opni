package etcd

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/core"
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
var _ storage.KeyringStoreBroker = (*EtcdStore)(nil)
var _ storage.KeyValueStoreBroker = (*EtcdStore)(nil)

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

func NewEtcdStore(ctx context.Context, conf *v1beta1.EtcdStorageSpec, opts ...EtcdStoreOption) *EtcdStore {
	options := EtcdStoreOptions{}
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
		Context:   ctx,
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
		EtcdStoreOptions: options,
		logger:           lg,
		client:           cli,
	}
}

func (e *EtcdStore) KeyringStore(ctx context.Context, prefix string, ref *core.Reference) (storage.KeyringStore, error) {
	return &etcdKeyringStore{
		client:    e.client,
		ref:       ref,
		prefix:    prefix,
		namespace: e.namespace,
	}, nil
}

func (e *EtcdStore) KeyValueStore(namespace string) (storage.KeyValueStore, error) {
	return &genericKeyValueStore{
		client:    e.client,
		namespace: namespace,
	}, nil
}
