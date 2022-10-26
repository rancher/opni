package gateway

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/dbason/featureflags"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	opensearchv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	opniv1beta2 "github.com/rancher/opni/apis/v1beta2"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/rancher/opni/plugins/logging/pkg/apis/opensearch"
	"github.com/rancher/opni/plugins/logging/pkg/backend"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers"
	"github.com/rancher/opni/plugins/logging/pkg/opensearchdata"
	corev1 "k8s.io/api/core/v1"
)

const (
	OpensearchBindingName = "opni-logging"
)

type Plugin struct {
	PluginOptions
	capabilityv1.UnsafeBackendServer
	opensearch.UnsafeOpensearchServer
	system.UnimplementedSystemPluginClient
	loggingadmin.UnsafeLoggingAdminServer
	ctx                 context.Context
	k8sClient           client.Client
	logger              *zap.SugaredLogger
	storageBackend      future.Future[storage.Backend]
	mgmtApi             future.Future[managementv1.ManagementClient]
	nodeManagerClient   future.Future[capabilityv1.NodeManagerClient]
	uninstallController future.Future[*task.Controller]
	opensearchManager   opensearchdata.Manager
	manageFlag          featureflags.FeatureFlag
	logging             backend.LoggingBackend
}

type PluginOptions struct {
	storageNamespace  string
	opensearchCluster *opnimeta.OpensearchClusterRef
	restconfig        *rest.Config
	featureOverride   featureflags.FeatureFlag
	version           string
	natsRef           *corev1.LocalObjectReference
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

func WithRestConfig(restconfig *rest.Config) PluginOption {
	return func(o *PluginOptions) {
		o.restconfig = restconfig
	}
}

func FeatureOverride(flagOverride featureflags.FeatureFlag) PluginOption {
	return func(o *PluginOptions) {
		o.featureOverride = flagOverride
	}
}
func WithVersion(version string) PluginOption {
	return func(o *PluginOptions) {
		o.version = version
	}
}

func WithNatsRef(ref *corev1.LocalObjectReference) PluginOption {
	return func(o *PluginOptions) {
		o.natsRef = ref
	}
}

func NewPlugin(ctx context.Context, opts ...PluginOption) *Plugin {
	options := PluginOptions{
		storageNamespace: os.Getenv("POD_NAMESPACE"),
	}
	options.apply(opts...)

	if options.natsRef == nil {
		options.natsRef = &corev1.LocalObjectReference{
			Name: "opni",
		}
	}

	lg := logger.NewPluginLogger().Named("logging")

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(opniv1beta2.AddToScheme(scheme))
	utilruntime.Must(opensearchv1.AddToScheme(scheme))
	utilruntime.Must(opnicorev1beta1.AddToScheme(scheme))
	utilruntime.Must(loggingv1beta1.AddToScheme(scheme))

	var restconfig *rest.Config
	if options.restconfig != nil {
		restconfig = options.restconfig
	} else {
		restconfig = ctrl.GetConfigOrDie()
	}

	cli, err := client.New(restconfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		lg.Error(fmt.Sprintf("failed to create k8s client: %v", err))
		os.Exit(1)
	}

	p := &Plugin{
		PluginOptions:       options,
		ctx:                 ctx,
		k8sClient:           cli,
		logger:              lg,
		storageBackend:      future.New[storage.Backend](),
		mgmtApi:             future.New[managementv1.ManagementClient](),
		uninstallController: future.New[*task.Controller](),
		opensearchManager: *opensearchdata.NewManager(
			lg.Named("opensearch-manager"),
		),
		nodeManagerClient: future.New[capabilityv1.NodeManagerClient](),
	}

	future.Wait4(p.storageBackend, p.mgmtApi, p.uninstallController, p.nodeManagerClient,
		func(
			storageBackend storage.Backend,
			mgmtClient managementv1.ManagementClient,
			uninstallController *task.Controller,
			nodeManagerClient capabilityv1.NodeManagerClient,
		) {
			clusterDriver, _ := drivers.NewKubernetesManagerDriver(
				drivers.WithK8sClient(p.k8sClient),
				drivers.WithLogger(*p.logger.Named("cluster-driver")),
				drivers.WithOpensearchCluster(p.opensearchCluster),
			)
			p.logging.Initialize(backend.LoggingBackendConfig{
				Logger:              p.logger.Named("logging-backend"),
				StorageBackend:      storageBackend,
				UninstallController: uninstallController,
				OpensearchCluster:   p.opensearchCluster,
				MgmtClient:          mgmtClient,
				NodeManagerClient:   nodeManagerClient,
				ClusterDriver:       clusterDriver,
			})
		},
	)

	return p
}

var _ loggingadmin.LoggingAdminServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeGateway))

	ns := os.Getenv("POD_NAMESPACE")

	opniCluster := &opnimeta.OpensearchClusterRef{
		Name:      "opni",
		Namespace: ns,
	}

	p := NewPlugin(
		ctx,
		WithNamespace(ns),
		WithOpensearchCluster(opniCluster),
	)
	p.logger.Info("logging plugin enabled")

	restconfig, err := rest.InClusterConfig()
	if err != nil {
		if !errors.Is(err, rest.ErrNotInCluster) {
			p.logger.Fatalf("failed to create config: %s", err)
		}
	}

	if p.restconfig != nil {
		restconfig = p.restconfig
	}

	if restconfig != nil {
		features.PopulateFeatures(ctx, restconfig)
		p.manageFlag = features.FeatureList.GetFeature("manage-opensearch")
	}

	if p.featureOverride != nil {
		p.manageFlag = p.featureOverride
	}

	go p.opensearchManager.SetClient(p.setOpensearchClient)

	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(&p.logging))
	scheme.Add(streamext.StreamAPIExtensionPluginID, streamext.NewPlugin(p))

	if restconfig != nil && p.manageFlag != nil && p.manageFlag.IsEnabled() {
		scheme.Add(managementext.ManagementAPIExtensionPluginID,
			managementext.NewPlugin(util.PackService(&loggingadmin.LoggingAdmin_ServiceDesc, p)))
	}

	return scheme
}
