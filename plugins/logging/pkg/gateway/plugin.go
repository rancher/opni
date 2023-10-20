package gateway

import (
	"context"
	"errors"
	"fmt"
	"os"

	"log/slog"

	"github.com/dbason/featureflags"
	"github.com/rancher/opni/plugins/logging/apis/loggingadmin"
	"github.com/rancher/opni/plugins/logging/apis/opensearch"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	"github.com/rancher/opni/pkg/agent"
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
	alertingApi "github.com/rancher/opni/plugins/logging/apis/alerting"
	"github.com/rancher/opni/plugins/logging/pkg/backend"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/alerting"
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
	ctx                 context.Context
	logger              *slog.Logger
	storageBackend      future.Future[storage.Backend]
	kv                  future.Future[system.KeyValueStoreClient]
	mgmtApi             future.Future[managementv1.ManagementClient]
	delegate            future.Future[streamext.StreamDelegate[agent.ClientSet]]
	uninstallController future.Future[*task.Controller]
	alertingServer      *alerting.AlertingManagementServer
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

	lg := logger.NewPluginLogger().WithGroup("logging")

	kv := future.New[system.KeyValueStoreClient]()

	p := &Plugin{
		PluginOptions:       options,
		ctx:                 ctx,
		logger:              lg,
		storageBackend:      future.New[storage.Backend](),
		mgmtApi:             future.New[managementv1.ManagementClient](),
		uninstallController: future.New[*task.Controller](),
		kv:                  kv,
		alertingServer:      alerting.NewAlertingManagementServer(),
		opensearchManager: opensearchdata.NewManager(
			lg.WithGroup("opensearch-manager"),
			kv,
		),
		delegate: future.New[streamext.StreamDelegate[agent.ClientSet]](),
		otelForwarder: otel.NewOTELForwarder(
			otel.WithLogger(lg.WithGroup("otel-forwarder")),
			otel.WithAddress(fmt.Sprintf(
				"%s:%d",
				preprocessor.PreprocessorServiceName(opniopensearch.OpniPreprocessingInstanceName),
				OpniPreprocessingPort,
			)),
			otel.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
		),
	}

	future.Wait4(p.storageBackend, p.mgmtApi, p.uninstallController, p.delegate,
		func(
			storageBackend storage.Backend,
			mgmtClient managementv1.ManagementClient,
			uninstallController *task.Controller,
			delegate streamext.StreamDelegate[agent.ClientSet],
		) {
			p.logging.Initialize(backend.LoggingBackendConfig{
				Logger:              p.logger.WithGroup("logging-backend"),
				StorageBackend:      storageBackend,
				UninstallController: uninstallController,
				MgmtClient:          mgmtClient,
				Delegate:            delegate,
				OpensearchManager:   p.opensearchManager,
				ClusterDriver:       p.backendDriver,
			})
		},
	)

	return p
}

var (
	_ loggingadmin.LoggingAdminV2Server = (*LoggingManagerV2)(nil)
	_ collogspb.LogsServiceServer       = (*otel.OTELForwarder)(nil)
)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeGateway))

	p := NewPlugin(ctx)
	p.logger.Info("logging plugin enabled")

	restconfig, err := rest.InClusterConfig()
	if err != nil {
		if !errors.Is(err, rest.ErrNotInCluster) {
			p.logger.Error(fmt.Sprintf("failed to create config: %s", err))
			os.Exit(1)
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
		p.logger.Error(fmt.Sprintf("could not find backend driver %q", driverName))
		os.Exit(1)
	}
	managementDriverBuilder, ok := managementdriver.Drivers.Get(driverName)
	if !ok {
		p.logger.Error(fmt.Sprintf("could not find management driver %q", driverName))
		os.Exit(1)
	}

	driverOptions := []driverutil.Option{
		driverutil.NewOption("restConfig", p.restconfig),
		driverutil.NewOption("namespace", p.storageNamespace),
		driverutil.NewOption("opensearchCluster", p.opensearchCluster),
		driverutil.NewOption("logger", p.logger),
	}
	p.backendDriver, err = backendDriverBuilder(ctx, driverOptions...)
	if err != nil {
		p.logger.Error(fmt.Sprintf("failed to create backend driver: %v", err))
		os.Exit(1)
	}

	p.managementDriver, err = managementDriverBuilder(ctx, driverOptions...)
	if err != nil {
		p.logger.Error(fmt.Sprintf("failed to create management driver: %v", err))
		os.Exit(1)
	}

	loggingManager := p.NewLoggingManagerForPlugin()

	if state := p.backendDriver.GetInstallStatus(ctx); state == backenddriver.Installed {
		go p.opensearchManager.SetClient(loggingManager.managementDriver.NewOpensearchClientForCluster)
		go p.alertingServer.SetClient(loggingManager.managementDriver.NewOpensearchClientForCluster)
		err = loggingManager.createInitialAdmin()
		if err != nil {
			p.logger.Warn(fmt.Sprintf("failed to create initial admin: %v", err))
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
			util.PackService(&alertingApi.MonitorManagement_ServiceDesc, p.alertingServer),
			util.PackService(&alertingApi.NotificationManagement_ServiceDesc, p.alertingServer),
			util.PackService(&alertingApi.AlertManagement_ServiceDesc, p.alertingServer),
		),
	)

	return scheme
}

func (p *Plugin) NewLoggingManagerForPlugin() *LoggingManagerV2 {
	return &LoggingManagerV2{
		managementDriver:  p.managementDriver,
		backendDriver:     p.backendDriver,
		logger:            p.logger.WithGroup("opensearch-manager"),
		alertingServer:    p.alertingServer,
		opensearchManager: p.opensearchManager,
		storageNamespace:  p.storageNamespace,
		natsRef:           p.natsRef,
		otelForwarder:     p.otelForwarder,
		k8sObjectsName: func() string {
			if p.opensearchCluster == nil {
				return "opni"
			}
			return p.opensearchCluster.Name
		}(),
	}
}
