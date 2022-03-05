package etcd

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	defaultEtcdTimeout = 5 * time.Second
	retryErr           = errors.New("the object has been modified, retrying")
	defaultBackoff     = wait.Backoff{
		Steps:    20,
		Duration: 10 * time.Millisecond,
		Cap:      1 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
	}
)

func isRetryErr(err error) bool {
	return errors.Is(err, retryErr)
}

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

var _ storage.Backend = (*EtcdStore)(nil)

type EtcdStoreOptions struct {
	prefix string
}

type EtcdStoreOption func(*EtcdStoreOptions)

func (o *EtcdStoreOptions) Apply(opts ...EtcdStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithPrefix(prefix string) EtcdStoreOption {
	return func(o *EtcdStoreOptions) {
		o.prefix = prefix
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
	pfx := e.prefix
	if prefix != "" {
		pfx = prefix
	}
	return &etcdKeyringStore{
		client: e.client,
		ref:    ref,
		prefix: pfx,
	}, nil
}

func (e *EtcdStore) KeyValueStore(prefix string) (storage.KeyValueStore, error) {
	pfx := e.prefix
	if prefix != "" {
		pfx = prefix
	}
	return &genericKeyValueStore{
		client: e.client,
		prefix: pfx,
	}, nil
}
