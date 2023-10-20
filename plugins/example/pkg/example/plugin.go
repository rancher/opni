package example

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
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
	UnsafeExampleAPIExtensionServer
	UnsafeExampleUnaryExtensionServer
	capabilityv1.UnsafeBackendServer
	capabilityv1.UnsafeRBACManagerServer
	system.UnimplementedSystemPluginClient
	ctx    context.Context
	logger *zap.SugaredLogger

	storageBackend      future.Future[storage.Backend]
	rbacStorage         future.Future[storage.RoleStore]
	uninstallController future.Future[*task.Controller]
	driver              ExampleDriver
}

// ManagementServices implements managementext.ManagementAPIExtension.
func (p *ExamplePlugin) ManagementServices(ctrl managementext.ServiceController) []util.ServicePackInterface {
	ctrl.SetServingStatus(ExampleAPIExtension_ServiceDesc.ServiceName, managementext.NotServing)
	ctrl.SetServingStatus(Config_ServiceDesc.ServiceName, managementext.NotServing)

	return []util.ServicePackInterface{
		util.PackService[ExampleAPIExtensionServer](&ExampleAPIExtension_ServiceDesc, p),
		util.PackService[ConfigServer](&Config_ServiceDesc, &p.driver),
	}
}

var _ ExampleAPIExtensionServer = (*ExamplePlugin)(nil)
var _ ExampleUnaryExtensionServer = (*ExamplePlugin)(nil)

func (p *ExamplePlugin) Echo(_ context.Context, req *EchoRequest) (*EchoResponse, error) {
	return &EchoResponse{
		Message: req.Message,
	}, nil
}

func (p *ExamplePlugin) Hello(context.Context, *emptypb.Empty) (*EchoResponse, error) {
	return &EchoResponse{
		Message: "Hello World",
	}, nil
}

func (p *ExamplePlugin) Ready(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (p *ExamplePlugin) UseCachingProvider(cacheProvider caching.CachingProvider[proto.Message]) {
	cacheProvider.SetCache(caching.NewInMemoryGrpcTtlCache(50*1024*1024, 1*time.Minute))
}

func (p *ExamplePlugin) UseManagementAPI(client managementv1.ManagementClient) {
	cfg, err := client.GetConfig(context.Background(), &emptypb.Empty{}, grpc.WaitForReady(true))
	if err != nil {
		p.logger.With(zap.Error(err)).Error("failed to get config")
		return
	}
	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		p.logger.With(zap.Error(err)).Error("failed to load config")
		return
	}
	machinery.LoadAuthProviders(p.ctx, objectList)
	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		backend, err := machinery.ConfigureStorageBackend(p.ctx, &config.Spec.Storage)
		if err != nil {
			p.logger.With(zap.Error(err)).Error("failed to configure storage backend")
			return
		}
		p.storageBackend.Set(backend)
	})

	if !p.storageBackend.IsSet() {
		return
	}
	<-p.ctx.Done()
}

func (p *ExamplePlugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	ctrl, err := task.NewController(p.ctx, "uninstall", system.NewKVStoreClient[*corev1.TaskStatus](client), &uninstallTaskRunner{
		storageBackend: p.storageBackend.Get(),
	})
	if err != nil {
		p.logger.With(zap.Error(err)).Error("failed to create uninstall controller")
		return
	}
	p.uninstallController.Set(ctrl)

	p.driver.Initialize(ExampleDriverImplOptions{
		DefaultConfigStore: kvutil.WithKey(system.NewKVStoreClient[*ConfigSpec](client), "/config/default"),
		ActiveConfigStore:  kvutil.WithKey(system.NewKVStoreClient[*ConfigSpec](client), "/config/active"),
	})

	p.rbacStorage.Set(kvutil.WithPrefix(system.NewKVStoreClient[*corev1.Role](client), "/roles"))

	<-p.ctx.Done()
}

func (p *ExamplePlugin) ConfigureRoutes(app *gin.Engine) {
	app.GET("/example", func(c *gin.Context) {
		p.logger.Debug("handling /example")
		c.JSON(http.StatusOK, map[string]string{
			"message": "hello world",
		})
	})
}

func (p *ExamplePlugin) info() *capabilityv1.Details {
	return &capabilityv1.Details{
		Name:   wellknown.CapabilityExample,
		Source: "plugin_example",
	}
}

func (p *ExamplePlugin) Info(context.Context, *corev1.Reference) (*capabilityv1.Details, error) {
	return p.info(), nil
}

func (p *ExamplePlugin) List(context.Context, *emptypb.Empty) (*capabilityv1.DetailsList, error) {
	return &capabilityv1.DetailsList{
		Items: []*capabilityv1.Details{p.info()},
	}, nil
}

func (p *ExamplePlugin) CanInstall(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (p *ExamplePlugin) Status(ctx context.Context, ref *capabilityv1.StatusRequest) (*capabilityv1.NodeCapabilityStatus, error) {
	cluster, err := p.storageBackend.Get().GetCluster(ctx, ref.GetAgent())
	if err != nil {
		return nil, err
	}
	return &capabilityv1.NodeCapabilityStatus{
		Enabled: capabilities.Has(cluster, capabilities.Cluster(wellknown.CapabilityExample)),
	}, nil
}

func (p *ExamplePlugin) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	_, err := p.storageBackend.Get().UpdateCluster(ctx, req.Agent,
		storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityExample)),
	)
	if err != nil {
		return nil, err
	}
	return &capabilityv1.InstallResponse{
		Status: capabilityv1.InstallResponseStatus_Success,
	}, nil
}

func (p *ExamplePlugin) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	cluster, err := p.storageBackend.Get().GetCluster(ctx, req.Agent)
	if err != nil {
		return nil, err
	}
	if cluster == nil {
		return nil, status.Errorf(codes.NotFound, "cluster %q not found", req.Agent)
	}

	found := false
	_, err = p.storageBackend.Get().UpdateCluster(ctx, cluster.Reference(), func(c *corev1.Cluster) {
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

	err = p.uninstallController.Get().LaunchTask(req.Agent.Id)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (p *ExamplePlugin) UninstallStatus(_ context.Context, ref *capabilityv1.UninstallStatusRequest) (*corev1.TaskStatus, error) {
	return p.uninstallController.Get().TaskStatus(ref.GetAgent().GetId())
}

func (p *ExamplePlugin) CancelUninstall(_ context.Context, ref *capabilityv1.CancelUninstallRequest) (*emptypb.Empty, error) {
	p.uninstallController.Get().CancelTask(ref.GetAgent().GetId())
	return &emptypb.Empty{}, nil
}

func (p *ExamplePlugin) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InstallerTemplate not implemented")
}

func (p *ExamplePlugin) GetAvailablePermissions(_ context.Context, _ *emptypb.Empty) (*corev1.AvailablePermissions, error) {
	return &corev1.AvailablePermissions{
		Items: []*corev1.PermissionDescription{
			{
				Type: string(corev1.PermissionTypeCluster),
				Verbs: []*corev1.PermissionVerb{
					corev1.VerbGet(),
				},
			},
		},
	}, nil
}

func (p *ExamplePlugin) GetRole(ctx context.Context, in *corev1.Reference) (*corev1.Role, error) {
	var revision int64
	role, err := p.rbacStorage.Get().Get(ctx, in.GetId(), storage.WithRevisionOut(&revision))
	if err != nil {
		return nil, err
	}
	metadata := &corev1.RoleMetadata{
		ResourceVersion: strconv.FormatInt(revision, 10),
	}

	role.Metadata = metadata

	return role, nil
}

func (p *ExamplePlugin) CreateRole(ctx context.Context, in *corev1.Role) (*emptypb.Empty, error) {
	_, err := p.rbacStorage.Get().Get(ctx, in.Reference().GetId())
	if err == nil {
		return nil, storage.ErrAlreadyExists
	}
	if !storage.IsNotFound(err) {
		return nil, err
	}
	err = p.rbacStorage.Get().Put(ctx, in.GetId(), in)
	return &emptypb.Empty{}, err
}

func (p *ExamplePlugin) UpdateRole(ctx context.Context, in *corev1.Role) (*emptypb.Empty, error) {
	store := p.rbacStorage.Get()
	oldRole, err := store.Get(ctx, in.Reference().GetId())
	if err != nil {
		return &emptypb.Empty{}, err
	}

	revision, err := strconv.ParseInt(in.GetMetadata().GetResourceVersion(), 10, 64)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	oldRole.Permissions = in.GetPermissions()
	err = store.Put(ctx, oldRole.Reference().GetId(), oldRole, storage.WithRevision(revision))
	return &emptypb.Empty{}, err
}

func (p *ExamplePlugin) DeleteRole(ctx context.Context, in *corev1.Reference) (*emptypb.Empty, error) {
	err := p.rbacStorage.Get().Delete(ctx, in.GetId())
	return &emptypb.Empty{}, err
}

func (p *ExamplePlugin) ListRoles(ctx context.Context, _ *emptypb.Empty) (*corev1.RoleList, error) {
	keys, err := p.rbacStorage.Get().ListKeys(ctx, "")
	if err != nil {
		return nil, err
	}

	roles := []*corev1.Reference{}
	for _, key := range keys {
		roles = append(roles, &corev1.Reference{
			Id: key,
		})
	}
	return &corev1.RoleList{
		Items: roles,
	}, nil
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := &ExamplePlugin{
		ctx:                 ctx,
		logger:              logger.NewPluginLogger().Named("example"),
		storageBackend:      future.New[storage.Backend](),
		rbacStorage:         future.New[storage.RoleStore](),
		uninstallController: future.New[*task.Controller](),
	}
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(p))
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(p))
	scheme.Add(capability.CapabilityRBACPluginID, capability.NewRBACPlugin(p))
	return scheme
}

type uninstallTaskRunner struct {
	uninstall.DefaultPendingHandler

	storageBackend storage.Backend
}

func (u *uninstallTaskRunner) OnTaskRunning(ctx context.Context, ti task.ActiveTask) error {
	ti.AddLogEntry(zapcore.InfoLevel, "Removing capability from cluster metadata")
	_, err := u.storageBackend.UpdateCluster(ctx, &corev1.Reference{
		Id: ti.TaskId(),
	}, storage.NewRemoveCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityExample)))
	if err != nil {
		return err
	}
	return nil
}

func (u *uninstallTaskRunner) OnTaskCompleted(ctx context.Context, ti task.ActiveTask, state task.State, args ...any) {
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
	_, err := u.storageBackend.UpdateCluster(ctx, &corev1.Reference{
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
