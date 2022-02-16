package etcd

import (
	"context"
	"crypto/tls"
	"encoding/base64"
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

type clusterKeyringStore struct {
	client     *clientv3.Client
	clusterRef *core.Reference
	namespace  string
}

type genericKeyValueStore struct {
	client    *clientv3.Client
	namespace string
}

func (e *EtcdStore) KeyringStore(ctx context.Context, ref *core.Reference) (storage.KeyringStore, error) {
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

func (e *EtcdStore) NewKeyValueStore(namespace string) (storage.KeyValueStore, error) {
	return &genericKeyValueStore{
		client:    e.client,
		namespace: namespace,
	}, nil
}

func (s *genericKeyValueStore) Put(ctx context.Context, key string, value []byte) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	_, err := s.client.Put(ctx, path.Join(s.namespace, key), base64.StdEncoding.EncodeToString(value))
	if err != nil {
		return err
	}
	return nil
}

func (s *genericKeyValueStore) Get(ctx context.Context, key string) ([]byte, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := s.client.Get(ctx, path.Join(s.namespace, key))
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, storage.ErrNotFound
	}
	return base64.StdEncoding.DecodeString(string(resp.Kvs[0].Value))
}

func (s *genericKeyValueStore) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := s.client.Get(ctx, path.Join(s.namespace, prefix),
		clientv3.WithPrefix(),
		clientv3.WithKeysOnly(),
	)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		keys[i] = string(kv.Key)
	}
	return keys, nil
}
