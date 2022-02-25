package secrets

import (
	"context"
	"os"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/sdk/api"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SecretsStore struct {
	SecretsStoreOptions
	client client.Client
	logger *zap.SugaredLogger
}

type SecretsStoreOptions struct {
	namespace  string
	restConfig *rest.Config
}

type SecretsStoreOption func(*SecretsStoreOptions)

func (o *SecretsStoreOptions) Apply(opts ...SecretsStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithNamespace(ns string) SecretsStoreOption {
	return func(o *SecretsStoreOptions) {
		o.namespace = ns
	}
}

func WithRestConfig(rc *rest.Config) SecretsStoreOption {
	return func(o *SecretsStoreOptions) {
		o.restConfig = rc
	}
}

func NewSecretsStore(opts ...SecretsStoreOption) *SecretsStore {
	lg := logger.New().Named("crd-store")

	options := SecretsStoreOptions{
		namespace: os.Getenv("POD_NAMESPACE"),
	}
	options.Apply(opts...)
	if options.namespace == "" {
		lg.Warn("namespace is not set, using \"default\"")
		options.namespace = "default"
	}
	if options.restConfig == nil {
		options.restConfig = util.Must(rest.InClusterConfig())
	}
	return &SecretsStore{
		SecretsStoreOptions: options,
		client: util.Must(client.New(options.restConfig, client.Options{
			Scheme: api.NewScheme(),
		})),
		logger: lg,
	}
}

func (c *SecretsStore) KeyringStore(ctx context.Context, prefix string, ref *core.Reference) (storage.KeyringStore, error) {
	return &secretKeyringStore{
		client:       c.client,
		prefix:       prefix,
		k8sNamespace: c.namespace,
		ref:          ref,
	}, nil
}

func (c *SecretsStore) KeyValueStore(prefix string) (storage.KeyValueStore, error) {
	return &secretKeyValueStore{
		client:       c.client,
		prefix:       prefix,
		k8sNamespace: c.namespace,
	}, nil
}
