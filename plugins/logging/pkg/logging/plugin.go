package logging

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util"
	opniv2beta1 "github.com/rancher/opni/apis/v2beta1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	opensearchv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	OpensearchBindingName = "opni-logging"
)

type Plugin struct {
	PluginOptions
	ctx            context.Context
	k8sClient      client.Client
	logger         hclog.Logger
	storageBackend *util.Future[storage.Backend]
	mgmtApi        *util.Future[management.ManagementClient]
}

type PluginOptions struct {
	storageNamespace  string
	opensearchCluster *opniv2beta1.OpensearchClusterRef
}

type PluginOption func(*PluginOptions)

func (o *PluginOptions) Apply(opts ...PluginOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithNamespace(namespace string) PluginOption {
	return func(o *PluginOptions) {
		o.storageNamespace = namespace
	}
}

func WithOpensearchCluster(cluster *opniv2beta1.OpensearchClusterRef) PluginOption {
	return func(o *PluginOptions) {
		o.opensearchCluster = cluster
	}
}

func NewPlugin(ctx context.Context, opts ...PluginOption) *Plugin {
	options := PluginOptions{}
	options.Apply(opts...)

	lg := logger.NewForPlugin()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(opniv2beta1.AddToScheme(scheme))
	utilruntime.Must(opensearchv1.AddToScheme(scheme))

	cli, err := client.New(ctrl.GetConfigOrDie(), client.Options{
		Scheme: scheme,
	})
	if err != nil {
		lg.Error(fmt.Sprintf("failed to create k8s client: %v", err))
		os.Exit(1)
	}

	return &Plugin{
		PluginOptions:  options,
		ctx:            ctx,
		k8sClient:      cli,
		logger:         lg,
		storageBackend: util.NewFuture[storage.Backend](),
		mgmtApi:        util.NewFuture[management.ManagementClient](),
	}
}
