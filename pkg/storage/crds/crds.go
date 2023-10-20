package crds

import (
	"fmt"
	"os"
	"time"

	"github.com/rancher/opni/apis"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"log/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
)

type CRDStore struct {
	CRDStoreOptions
	client client.WithWatch
	logger *slog.Logger
}

var _ storage.TokenStore = (*CRDStore)(nil)
var _ storage.RBACStore = (*CRDStore)(nil)
var _ storage.KeyringStoreBroker = (*CRDStore)(nil)

var (
	defaultBackoff = wait.Backoff{
		Steps:    20,
		Duration: 10 * time.Millisecond,
		Cap:      1 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
	}
)

type CRDStoreOptions struct {
	namespace  string
	restConfig *rest.Config
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

func NewCRDStore(opts ...CRDStoreOption) *CRDStore {
	lg := logger.New().WithGroup("crd-store")
	options := CRDStoreOptions{
		namespace: os.Getenv("POD_NAMESPACE"),
	}
	options.apply(opts...)
	if options.namespace == "" {
		lg.Warn("namespace is not set, using \"default\"")
		options.namespace = "default"
	}
	if options.restConfig == nil {
		options.restConfig = util.Must(rest.InClusterConfig())
	}
	options.restConfig.QPS = 50
	options.restConfig.Burst = 100
	s := apis.NewScheme()
	return &CRDStore{
		CRDStoreOptions: options,
		client: util.Must(client.NewWithWatch(options.restConfig, client.Options{
			Scheme: s,
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

func init() {
	storage.RegisterStoreBuilder(v1beta1.StorageTypeCRDs, func(args ...any) (any, error) {
		var opts []CRDStoreOption
		for _, arg := range args {
			switch arg := arg.(type) {
			case string:
				opts = append(opts, WithNamespace(arg))
			case *rest.Config:
				opts = append(opts, WithRestConfig(arg))
			default:
				return nil, fmt.Errorf("unexpected argument %v", arg)
			}
		}
		return NewCRDStore(opts...), nil
	})
}
