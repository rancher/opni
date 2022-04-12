package crds

import (
	"context"
	"os"
	"time"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/sdk/api"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CRDStore struct {
	CRDStoreOptions
	client client.Client
	logger *zap.SugaredLogger
}

var _ storage.TokenStore = (*CRDStore)(nil)
var _ storage.ClusterStore = (*CRDStore)(nil)
var _ storage.RBACStore = (*CRDStore)(nil)
var _ storage.KeyringStoreBroker = (*CRDStore)(nil)

type CRDStoreOptions struct {
	namespace      string
	restConfig     *rest.Config
	commandTimeout time.Duration
}

type CRDStoreOption func(*CRDStoreOptions)

func (o *CRDStoreOptions) Apply(opts ...CRDStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithNamespace(ns string) CRDStoreOption {
	return func(o *CRDStoreOptions) {
		o.namespace = ns
	}
}

func WithRestConfig(rc *rest.Config) CRDStoreOption {
	return func(o *CRDStoreOptions) {
		o.restConfig = rc
	}
}

func WithCommandTimeout(timeout time.Duration) CRDStoreOption {
	return func(o *CRDStoreOptions) {
		o.commandTimeout = timeout
	}
}

func NewCRDStore(opts ...CRDStoreOption) *CRDStore {
	lg := logger.New().Named("crd-store")
	options := CRDStoreOptions{
		namespace:      os.Getenv("POD_NAMESPACE"),
		commandTimeout: 5 * time.Second,
	}
	options.Apply(opts...)
	if options.namespace == "" {
		lg.Warn("namespace is not set, using \"default\"")
		options.namespace = "default"
	}
	if options.restConfig == nil {
		options.restConfig = util.Must(rest.InClusterConfig())
	}
	options.restConfig.Timeout = options.commandTimeout
	return &CRDStore{
		CRDStoreOptions: options,
		client: util.Must(client.New(options.restConfig, client.Options{
			Scheme: api.NewScheme(),
		})),
		logger: lg,
	}
}

func (e *CRDStore) KeyringStore(ctx context.Context, prefix string, ref *core.Reference) (storage.KeyringStore, error) {
	return &crdKeyringStore{
		CRDStoreOptions: e.CRDStoreOptions,
		client:          e.client,
		ref:             ref,
		prefix:          prefix,
	}, nil
}
