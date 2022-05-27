package etcd

import (
	"context"
	"crypto/tls"
	"errors"
	"path"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	retryErr       = errors.New("the object has been modified, retrying")
	defaultBackoff = wait.Backoff{
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
	Logger *zap.SugaredLogger
	Client *clientv3.Client
}

var _ storage.Backend = (*EtcdStore)(nil)

type EtcdStoreOptions struct {
	Prefix         string
	CommandTimeout time.Duration
}

type EtcdStoreOption func(*EtcdStoreOptions)

func (o *EtcdStoreOptions) apply(opts ...EtcdStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithPrefix(prefix string) EtcdStoreOption {
	return func(o *EtcdStoreOptions) {
		o.Prefix = prefix
	}
}

func WithCommandTimeout(timeout time.Duration) EtcdStoreOption {
	return func(o *EtcdStoreOptions) {
		o.CommandTimeout = timeout
	}
}

func NewEtcdStore(ctx context.Context, conf *v1beta1.EtcdStorageSpec, opts ...EtcdStoreOption) *EtcdStore {
	options := EtcdStoreOptions{
		CommandTimeout: 5 * time.Second,
	}
	options.apply(opts...)
	lg := logger.New(logger.WithLogLevel(zap.WarnLevel)).Named("etcd")
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
		Logger:    lg.Desugar(),
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
		Logger:           lg,
		Client:           cli,
	}
}

func (e *EtcdStore) KeyringStore(ctx context.Context, prefix string, ref *corev1.Reference) (storage.KeyringStore, error) {
	pfx := e.Prefix
	if prefix != "" {
		pfx = prefix
	}
	return &etcdKeyringStore{
		EtcdStoreOptions: e.EtcdStoreOptions,
		client:           e.Client,
		ref:              ref,
		prefix:           pfx,
	}, nil
}

func (e *EtcdStore) KeyValueStore(prefix string) (storage.KeyValueStore, error) {
	pfx := e.Prefix
	if prefix != "" {
		pfx = prefix
	}
	return &genericKeyValueStore{
		EtcdStoreOptions: e.EtcdStoreOptions,
		client:           e.Client,
		prefix:           path.Join(pfx, "kv"),
	}, nil
}
