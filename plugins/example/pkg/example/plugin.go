package example

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/kvutil"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ExamplePlugin struct {
	util.Initializer
	UnsafeExampleAPIExtensionServer
	UnsafeExampleUnaryExtensionServer
	capabilityv1.UnsafeBackendServer
	system.UnimplementedSystemPluginClient
	ctx    context.Context
	logger *zap.SugaredLogger

	storageBackend      future.Future[storage.Backend]
	uninstallController future.Future[*task.Controller]

	configServerBackend ConfigServerBackend
}

// ManagementServices implements managementext.ManagementAPIExtension.
func (p *ExamplePlugin) ManagementServices() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[ExampleAPIExtensionServer](&ExampleAPIExtension_ServiceDesc, p),
		util.PackService[ConfigServer](&Config_ServiceDesc, &p.configServerBackend),
	}
}

// UseServiceController implements managementext.ManagementAPIExtension.
func (p *ExamplePlugin) UseServiceController(sc managementext.ServiceController) {
	sc.SetServingStatus(ExampleAPIExtension_ServiceDesc.ServiceName, managementext.Serving)
	sc.SetServingStatus(Config_ServiceDesc.ServiceName, managementext.Serving)
}

var _ ExampleAPIExtensionServer = (*ExamplePlugin)(nil)
var _ ExampleUnaryExtensionServer = (*ExamplePlugin)(nil)

func (s *ExamplePlugin) Initialize() {
	s.InitOnce(func() {})
}

func (s *ExamplePlugin) Echo(_ context.Context, req *EchoRequest) (*EchoResponse, error) {
	return &EchoResponse{
		Message: req.Message,
	}, nil
}

func (s *ExamplePlugin) Hello(context.Context, *emptypb.Empty) (*EchoResponse, error) {
	return &EchoResponse{
		Message: "Hello World",
	}, nil
}

func (s *ExamplePlugin) Ready(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if !s.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	return &emptypb.Empty{}, nil
}

func (s *ExamplePlugin) UseCachingProvider(cacheProvider caching.CachingProvider[proto.Message]) {
	cacheProvider.SetCache(caching.NewInMemoryGrpcTtlCache(50*1024*1024, 1*time.Minute))
}

func (s *ExamplePlugin) UseManagementAPI(client managementv1.ManagementClient) {
	cfg, err := client.GetConfig(context.Background(), &emptypb.Empty{}, grpc.WaitForReady(true))
	if err != nil {
		s.logger.With(zap.Error(err)).Error("failed to get config")
		return
	}
	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		s.logger.With(zap.Error(err)).Error("failed to load config")
		return
	}
	machinery.LoadAuthProviders(s.ctx, objectList)
	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		backend, err := machinery.ConfigureStorageBackend(s.ctx, &config.Spec.Storage)
		if err != nil {
			s.logger.With(zap.Error(err)).Error("failed to configure storage backend")
			return
		}
		s.storageBackend.Set(backend)
	})

	if !s.storageBackend.IsSet() {
		return
	}
	<-s.ctx.Done()
}

func (s *ExamplePlugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	ctrl, err := task.NewController(s.ctx, "uninstall", system.NewKVStoreClient[*corev1.TaskStatus](client), &uninstallTaskRunner{
		storageBackend: s.storageBackend.Get(),
	})
	if err != nil {
		s.logger.With(zap.Error(err)).Error("failed to create uninstall controller")
		return
	}
	s.uninstallController.Set(ctrl)

	builder, _ := drivers.Get("example")
	driver, _ := builder(s.ctx,
		driverutil.NewOption("defaultConfigStore", kvutil.WithKey(system.NewKVStoreClient[*ConfigSpec](client), "/config/default")),
		driverutil.NewOption("activeConfigStore", kvutil.WithKey(system.NewKVStoreClient[*ConfigSpec](client), "/config/active")),
	)
	s.configServerBackend.Initialize(driver)

	<-s.ctx.Done()
}

func (s *ExamplePlugin) ConfigureRoutes(app *gin.Engine) {
	app.GET("/example", func(c *gin.Context) {
		s.logger.Debug("handling /example")
		c.JSON(http.StatusOK, map[string]string{
			"message": "hello world",
		})
	})
}

func (s *ExamplePlugin) Info(context.Context, *emptypb.Empty) (*capabilityv1.Details, error) {
	return &capabilityv1.Details{
		Name:   wellknown.CapabilityExample,
		Source: "plugin_example",
	}, nil
}

func (s *ExamplePlugin) CanInstall(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *ExamplePlugin) Status(ctx context.Context, ref *corev1.Reference) (*capabilityv1.NodeCapabilityStatus, error) {
	cluster, err := s.storageBackend.Get().GetCluster(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &capabilityv1.NodeCapabilityStatus{
		Enabled: capabilities.Has(cluster, capabilities.Cluster(wellknown.CapabilityExample)),
	}, nil
}

func (s *ExamplePlugin) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	_, err := s.storageBackend.Get().UpdateCluster(ctx, req.Cluster,
		storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityExample)),
	)
	if err != nil {
		return nil, err
	}
	return &capabilityv1.InstallResponse{
		Status: capabilityv1.InstallResponseStatus_Success,
	}, nil
}

func (s *ExamplePlugin) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	cluster, err := s.storageBackend.Get().GetCluster(ctx, req.Cluster)
	if err != nil {
		return nil, err
	}
	if cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.Cluster)
	}

	found := false
	_, err = s.storageBackend.Get().UpdateCluster(ctx, cluster.Reference(), func(c *corev1.Cluster) {
		for _, cap := range c.Metadata.Capabilities {
			if cap.Name == wellknown.CapabilityExample {
				found = true
				cap.DeletionTimestamp = timestamppb.Now()
				break
			}
		}
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, status.Error(codes.FailedPrecondition, "cluster does not have the reuqested capability")
	}

	err = s.uninstallController.Get().LaunchTask(req.Cluster.Id)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *ExamplePlugin) UninstallStatus(_ context.Context, ref *corev1.Reference) (*corev1.TaskStatus, error) {
	return s.uninstallController.Get().TaskStatus(ref.GetId())
}

func (s *ExamplePlugin) CancelUninstall(_ context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	s.uninstallController.Get().CancelTask(ref.GetId())
	return &emptypb.Empty{}, nil
}

func (s *ExamplePlugin) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InstallerTemplate not implemented")
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := &ExamplePlugin{
		ctx:                 ctx,
		logger:              logger.NewPluginLogger().Named("example"),
		storageBackend:      future.New[storage.Backend](),
		uninstallController: future.New[*task.Controller](),
	}

	future.Wait2(p.storageBackend, p.uninstallController, func(_ storage.Backend, _ *task.Controller) {
		p.Initialize()
	})
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(p))
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(p))
	return scheme
}

type uninstallTaskRunner struct {
	uninstall.DefaultPendingHandler

	storageBackend storage.Backend
}

func (a *uninstallTaskRunner) OnTaskRunning(ctx context.Context, ti task.ActiveTask) error {
	ti.AddLogEntry(zapcore.InfoLevel, "Removing capability from cluster metadata")
	_, err := a.storageBackend.UpdateCluster(ctx, &corev1.Reference{
		Id: ti.TaskId(),
	}, storage.NewRemoveCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityExample)))
	if err != nil {
		return err
	}
	return nil
}

func (a *uninstallTaskRunner) OnTaskCompleted(ctx context.Context, ti task.ActiveTask, state task.State, args ...any) {

	switch state {
	case task.StateCompleted:
		ti.AddLogEntry(zapcore.InfoLevel, "Capability uninstalled successfully")
		return // no deletion timestamp to reset, since the capability should be gone
	case task.StateFailed:
		ti.AddLogEntry(zapcore.ErrorLevel, fmt.Sprintf("Capability uninstall failed: %v", args[0]))
	case task.StateCanceled:
		ti.AddLogEntry(zapcore.InfoLevel, "Capability uninstall canceled")
	}

	// Reset the deletion timestamp
	_, err := a.storageBackend.UpdateCluster(ctx, &corev1.Reference{
		Id: ti.TaskId(),
	}, func(c *corev1.Cluster) {
		for _, cap := range c.GetCapabilities() {
			if cap.Name == wellknown.CapabilityExample {
				cap.DeletionTimestamp = nil
				break
			}
		}
	})
	if err != nil {
		ti.AddLogEntry(zapcore.WarnLevel, fmt.Sprintf("Failed to reset deletion timestamp: %v", err))
	}
}
