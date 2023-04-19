package gateway

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/dbason/featureflags"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/resources/opniopensearch"
	"github.com/rancher/opni/pkg/resources/preprocessor"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/rancher/opni/plugins/logging/pkg/apis/opensearch"
	"github.com/rancher/opni/plugins/logging/pkg/backend"
	backenddriver "github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend"
	managementdriver "github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/management"
	"github.com/rancher/opni/plugins/logging/pkg/opensearchdata"
	"github.com/rancher/opni/plugins/logging/pkg/otel"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
)

const (
	OpensearchBindingName = "opni-logging"
	OpniPreprocessingPort = 4317
)

type Plugin struct {
	PluginOptions
	capabilityv1.UnsafeBackendServer
	opensearch.UnsafeOpensearchServer
	system.UnimplementedSystemPluginClient
	loggingadmin.UnsafeLoggingAdminServer
	ctx                 context.Context
	logger              *zap.SugaredLogger
	storageBackend      future.Future[storage.Backend]
	kv                  future.Future[system.KeyValueStoreClient]
	mgmtApi             future.Future[managementv1.ManagementClient]
	nodeManagerClient   future.Future[capabilityv1.NodeManagerClient]
	uninstallController future.Future[*task.Controller]
	opensearchManager   *opensearchdata.Manager
	logging             backend.LoggingBackend
	otelForwarder       *otel.OTELForwarder
	backendDriver       backenddriver.ClusterDriver
	managementDriver    managementdriver.ClusterDriver
}

type PluginOptions struct {
	storageNamespace  string
	opensearchCluster *opnimeta.OpensearchClusterRef
	restconfig        *rest.Config
	featureOverride   featureflags.FeatureFlag
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

	kv := future.New[system.KeyValueStoreClient]()

	p := &Plugin{
		PluginOptions:       options,
		ctx:                 ctx,
		logger:              lg,
		storageBackend:      future.New[storage.Backend](),
		mgmtApi:             future.New[managementv1.ManagementClient](),
		uninstallController: future.New[*task.Controller](),
		kv:                  kv,
		opensearchManager: opensearchdata.NewManager(
			lg.Named("opensearch-manager"),
			kv,
		),
		nodeManagerClient: future.New[capabilityv1.NodeManagerClient](),
		otelForwarder: otel.NewOTELForwarder(
			otel.WithLogger(lg.Named("otel-forwarder")),
			otel.WithAddress(fmt.Sprintf(
				"%s:%d",
				preprocessor.PreprocessorServiceName(opniopensearch.OpniPreprocessingInstanceName),
				OpniPreprocessingPort,
			)),
			otel.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
		),
	}

	future.Wait4(p.storageBackend, p.mgmtApi, p.uninstallController, p.nodeManagerClient,
		func(
			storageBackend storage.Backend,
			mgmtClient managementv1.ManagementClient,
			uninstallController *task.Controller,
			nodeManagerClient capabilityv1.NodeManagerClient,
		) {
			p.logging.Initialize(backend.LoggingBackendConfig{
				Logger:              p.logger.Named("logging-backend"),
				StorageBackend:      storageBackend,
				UninstallController: uninstallController,
				MgmtClient:          mgmtClient,
				NodeManagerClient:   nodeManagerClient,
				OpensearchManager:   p.opensearchManager,
				ClusterDriver:       p.backendDriver,
			})
		},
	)

	return p
}

var _ loggingadmin.LoggingAdminV2Server = (*LoggingManagerV2)(nil)
var _ collogspb.LogsServiceServer = (*otel.OTELForwarder)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeGateway))

	p := NewPlugin(ctx)
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

	var driverName string
	if restconfig != nil {
		features.PopulateFeatures(ctx, restconfig)
		driverName = "kubernetes-manager"
	} else {
		// TODO: need a way to configure this
		driverName = "mock-driver"
	}

	var ok bool
	backendDriverBuilder, ok := backenddriver.Drivers.Get(driverName)
	if !ok {
		p.logger.Fatalf("could not find backend driver %q", driverName)
	}
	managementDriverBuilder, ok := managementdriver.Drivers.Get(driverName)
	if !ok {
		p.logger.Fatalf("could not find management driver %q", driverName)
	}

	driverOptions := []driverutil.Option{
		driverutil.NewOption("restConfig", p.restconfig),
		driverutil.NewOption("namespace", p.storageNamespace),
		driverutil.NewOption("opensearchCluster", p.opensearchCluster),
		driverutil.NewOption("logger", p.logger),
	}
	p.backendDriver, err = backendDriverBuilder(ctx, driverOptions...)
	if err != nil {
		p.logger.Fatalf("failed to create backend driver: %v", err)
	}

	p.managementDriver, err = managementDriverBuilder(ctx, driverOptions...)
	if err != nil {
		p.logger.Fatalf("failed to create management driver: %v", err)
	}

	loggingManager := p.NewLoggingManagerForPlugin()

	if state := p.backendDriver.GetInstallStatus(ctx); state == backenddriver.Installed {
		go p.opensearchManager.SetClient(loggingManager.managementDriver.NewOpensearchClientForCluster)
		err = loggingManager.createInitialAdmin()
		if err != nil {
			p.logger.Warnf("failed to create initial admin: %v", err)
		}
		p.otelForwarder.BackgroundInitClient()
	}

	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(&p.logging))
	scheme.Add(streamext.StreamAPIExtensionPluginID, streamext.NewGatewayPlugin(p))
	scheme.Add(
		managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(
			util.PackService(&loggingadmin.LoggingAdminV2_ServiceDesc, loggingManager),
		),
	)

	return scheme
}

func (p *Plugin) NewLoggingManagerForPlugin() *LoggingManagerV2 {
	return &LoggingManagerV2{
		managementDriver:  p.managementDriver,
		backendDriver:     p.backendDriver,
		logger:            p.logger.Named("opensearch-manager"),
		opensearchManager: p.opensearchManager,
		storageNamespace:  p.storageNamespace,
		natsRef:           p.natsRef,
		otelForwarder:     p.otelForwarder,
	}
}

// func createKubernetesDrivers(
// 	restconfig *rest.Config,
// 	opensearchCluster *opnimeta.OpensearchClusterRef,
// 	lg *zap.SugaredLogger,
// ) (backenddriver.ClusterDriver, managementdriver.ClusterDriver, error) {
// 	cli, err := client.New(restconfig, client.Options{
// 		Scheme: apis.NewScheme(),
// 	})
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	backendDriver, err := kubernetes_manager.NewKubernetesManagerDriver(
// 		kubernetes_manager.WithK8sClient(cli),
// 		kubernetes_manager.WithLogger(lg),
// 		kubernetes_manager.WithOpensearchCluster(opensearchCluster),
// 	)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	managementDriver, err := kubernetes_manager2.NewKubernetesManagerDriver(
// 		kubernetes_manager2.WithOpensearchCluster(opensearchCluster),
// 		kubernetes_manager2.WithRestConfig(restconfig),
// 	)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	return backendDriver, managementDriver, nil
// }
