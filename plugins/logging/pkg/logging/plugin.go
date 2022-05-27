package logging

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"
	opniv1beta2 "github.com/rancher/opni/apis/v1beta2"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/logger"
	gatewayext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
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
	mgmtApi        *util.Future[managementv1.ManagementClient]
}

type PluginOptions struct {
	storageNamespace  string
	opensearchCluster *opnimeta.OpensearchClusterRef
}

type PluginOption func(*PluginOptions)

func (o *PluginOptions) apply(opts ...PluginOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithNamespace(namespace string) PluginOption {
	return func(o *PluginOptions) {
		o.storageNamespace = namespace
	}
}

func WithOpensearchCluster(cluster *opnimeta.OpensearchClusterRef) PluginOption {
	return func(o *PluginOptions) {
		o.opensearchCluster = cluster
	}
}

func NewPlugin(ctx context.Context, opts ...PluginOption) *Plugin {
	options := PluginOptions{}
	options.apply(opts...)

	lg := logger.NewForPlugin()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(opniv1beta2.AddToScheme(scheme))
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
		mgmtApi:        util.NewFuture[managementv1.ManagementClient](),
	}
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()

	opniCluster := &opnimeta.OpensearchClusterRef{
		Name:      "opni",
		Namespace: "opni-cluster-system",
	}

	p := NewPlugin(
		ctx,
		WithNamespace("opni-cluster-system"),
		WithOpensearchCluster(opniCluster),
	)

	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(gatewayext.GatewayAPIExtensionPluginID, gatewayext.NewPlugin(p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(wellknown.CapabilityLogs, p))
	return scheme
}
