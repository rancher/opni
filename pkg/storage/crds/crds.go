package crds

import (
	"os"

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

type CRDStoreOptions struct {
	namespace  string
	restConfig *rest.Config
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

func NewCRDStore(opts ...CRDStoreOption) *CRDStore {
	lg := logger.New().Named("crd-store")
	options := CRDStoreOptions{
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
	return &CRDStore{
		CRDStoreOptions: options,
		client: util.Must(client.New(options.restConfig, client.Options{
			Scheme: api.NewScheme(),
		})),
		logger: lg,
	}
}
