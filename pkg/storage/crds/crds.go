package crds

import (
	"os"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/opni/apis"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
)

type CRDStore struct {
	CRDStoreOptions
	client client.WithWatch
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

func (o *CRDStoreOptions) apply(opts ...CRDStoreOption) {
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
	options.apply(opts...)
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
		client: util.Must(client.NewWithWatch(options.restConfig, client.Options{
			Scheme: apis.NewScheme(),
		})),
		logger: lg,
	}
}

func (e *CRDStore) KeyringStore(prefix string, ref *corev1.Reference) storage.KeyringStore {
	return &crdKeyringStore{
		CRDStoreOptions: e.CRDStoreOptions,
		client:          e.client,
		ref:             ref,
		prefix:          prefix,
	}
}
