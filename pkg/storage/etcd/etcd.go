package etcd

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"path"
	"time"

	"log/slog"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/lestrrat-go/backoff/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
)

var (
	errRetry       = errors.New("the object has been modified, retrying")
	defaultBackoff = backoff.NewExponentialPolicy(
		backoff.WithMaxRetries(20),
		backoff.WithMinInterval(10*time.Millisecond),
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithJitterFactor(0.1),
		backoff.WithMultiplier(1.5),
	)
)

func isRetryErr(err error) bool {
	return errors.Is(err, errRetry)
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
	Logger *slog.Logger
	Client *clientv3.Client
}

var _ storage.Backend = (*EtcdStore)(nil)

type EtcdStoreOptions struct {
	Prefix string
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

func NewEtcdStore(ctx context.Context, conf *v1beta1.EtcdStorageSpec, opts ...EtcdStoreOption) (*EtcdStore, error) {
	options := EtcdStoreOptions{}
	options.apply(opts...)
	lg := logger.New(logger.WithLogLevel(slog.LevelWarn)).WithGroup("etcd")
	var tlsConfig *tls.Config
	if conf.Certs != nil {
		var err error
		tlsConfig, err = util.LoadClientMTLSConfig(conf.Certs)
		if err != nil {
			return nil, fmt.Errorf("failed to load client TLS config: %w", err)
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
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}
	lg.With(
		"endpoints", clientConfig.Endpoints,
	).Info("connecting to etcd")
	return &EtcdStore{
		EtcdStoreOptions: options,
		Logger:           lg,
		Client:           cli,
	}, nil
}

func (e *EtcdStore) KeyringStore(prefix string, ref *corev1.Reference) storage.KeyringStore {
	pfx := e.Prefix
	if prefix != "" {
		pfx = prefix
	}
	return &etcdKeyringStore{
		client: e.Client,
		ref:    ref,
		prefix: pfx,
	}
}

func (e *EtcdStore) KeyValueStore(prefix string) storage.KeyValueStore {
	pfx := e.Prefix
	if prefix != "" {
		pfx = prefix
	}
	return &genericKeyValueStore{
		client: e.Client,
		prefix: path.Join(pfx, "kv"),
	}
}

func init() {
	storage.RegisterStoreBuilder(v1beta1.StorageTypeEtcd, func(args ...any) (any, error) {
		ctx := args[0].(context.Context)
		conf := args[1].(*v1beta1.EtcdStorageSpec)

		var opts []EtcdStoreOption
		for _, arg := range args[2:] {
			switch v := arg.(type) {
			case string:
				opts = append(opts, WithPrefix(v))
			default:
				return nil, fmt.Errorf("unexpected argument: %v", arg)
			}
		}
		return NewEtcdStore(ctx, conf, opts...)
	})
}
