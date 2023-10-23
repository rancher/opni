package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/cortexproject/cortex/pkg/purger"
	"github.com/lestrrat-go/backoff/v2"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
	"golang.org/x/tools/pkg/memoize"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type CapabilityBackendService struct {
	Context types.ServiceContext `option:"context"`

	nodeStatusMu sync.RWMutex
	nodeStatus   map[string]*capabilityv1.NodeCapabilityStatus

	uninstallController *task.Controller
	nodeConfigClient    node.NodeConfigurationClient
}

var _ types.Service = (*CapabilityBackendService)(nil)

// AddToScheme implements types.PluginService
func (s *CapabilityBackendService) AddToScheme(scheme meta.Scheme) {
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(s))
}

func (s *CapabilityBackendService) StreamServices() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[node.NodeMetricsCapabilityServer](&node.NodeMetricsCapability_ServiceDesc, s),
	}
}

// Activate implements types.Service
func (s *CapabilityBackendService) Activate() error {
	uninstallRunner, err := NewUninstallTaskRunner(s.Context)
	if err != nil {
		return err
	}
	s.uninstallController, err = task.NewController(s.Context, "uninstall",
		system.NewKVStoreClient[*corev1.TaskStatus](s.Context.KeyValueStoreClient()), uninstallRunner)
	if err != nil {
		return err
	}

	s.nodeConfigClient = node.NewNodeConfigurationClient(s.Context.ExtensionClient().GetClientConnUnchecked())
	return nil
}

func (s *CapabilityBackendService) info() *capabilityv1.Details {
	return &capabilityv1.Details{
		Name:             wellknown.CapabilityMetrics,
		Source:           "plugin_metrics",
		AvailableDrivers: drivers.ClusterDrivers.List(),
		EnabledDriver:    s.Context.GatewayConfig().Spec.Cortex.Management.ClusterDriver,
	}
}

// Info implements capabilityv1.BackendServer
func (s *CapabilityBackendService) Info(_ context.Context, capability *corev1.Reference) (*capabilityv1.Details, error) {
	if capability.GetId() != wellknown.CapabilityMetrics {
		return nil, status.Errorf(codes.InvalidArgument, "capability %s not implemented by this plugin", capability.GetId())
	}
	return s.info(), nil
}

// List implements v1.BackendServer.
func (s *CapabilityBackendService) List(_ context.Context, _ *emptypb.Empty) (*capabilityv1.DetailsList, error) {
	return &capabilityv1.DetailsList{
		Items: []*capabilityv1.Details{
			s.info(),
		},
	}, nil
}

func (s *CapabilityBackendService) canInstall(ctx context.Context) error {
	stat, err := s.Context.ClusterDriver().Status(ctx, &emptypb.Empty{})
	if err != nil {
		return status.Error(codes.Unavailable, err.Error())
	}
	switch stat.InstallState {
	case driverutil.InstallState_Installed:
		// ok
	case driverutil.InstallState_NotInstalled, driverutil.InstallState_Uninstalling:
		return status.Error(codes.Unavailable, "cortex cluster is not installed")
	default:
		return status.Error(codes.Internal, "unknown cortex cluster state")
	}
	return nil
}

// Install implements capabilityv1.BackendServer
func (s *CapabilityBackendService) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	var warningErr error
	err := s.canInstall(ctx)
	if err != nil {
		if !req.IgnoreWarnings {
			return &capabilityv1.InstallResponse{
				Status:  capabilityv1.InstallResponseStatus_Error,
				Message: err.Error(),
			}, nil
		}
		warningErr = err
	}

	_, err = s.Context.StorageBackend().UpdateCluster(ctx, req.Agent,
		storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityMetrics)),
	)
	if err != nil {
		return nil, err
	}

	if err := RequestNodeSync(s.Context, req.Agent); err != nil {
		return &capabilityv1.InstallResponse{
			Status:  capabilityv1.InstallResponseStatus_Warning,
			Message: fmt.Errorf("sync request failed; agent may not be updated immediately: %v", err).Error(),
		}, nil
	}

	if warningErr != nil {
		return &capabilityv1.InstallResponse{
			Status:  capabilityv1.InstallResponseStatus_Warning,
			Message: warningErr.Error(),
		}, nil
	}
	return &capabilityv1.InstallResponse{
		Status: capabilityv1.InstallResponseStatus_Success,
	}, nil
}

func (s *CapabilityBackendService) Status(_ context.Context, req *capabilityv1.StatusRequest) (*capabilityv1.NodeCapabilityStatus, error) {
	s.nodeStatusMu.RLock()
	defer s.nodeStatusMu.RUnlock()

	if status, ok := s.nodeStatus[req.GetAgent().GetId()]; ok {
		return util.ProtoClone(status), nil
	}

	return nil, status.Error(codes.NotFound, "no status has been reported for this node")
}

// Uninstall implements capabilityv1.BackendServer
func (s *CapabilityBackendService) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	cluster, err := s.Context.ManagementClient().GetCluster(ctx, req.Agent)
	if err != nil {
		return nil, err
	}

	var defaultOpts capabilityv1.DefaultUninstallOptions
	if req.Options != nil {
		if err := defaultOpts.LoadFromStruct(req.Options); err != nil {
			return nil, fmt.Errorf("failed to unmarshal options: %v", err)
		}
	}

	exists := false
	for _, cap := range cluster.GetMetadata().GetCapabilities() {
		if cap.Name != wellknown.CapabilityMetrics {
			continue
		}
		exists = true

		// check for a previous stale task that may not have been cleaned up
		if cap.DeletionTimestamp != nil {
			// if the deletion timestamp is set and the task is not completed, error
			stat, err := s.uninstallController.TaskStatus(cluster.Id)
			if err != nil {
				if util.StatusCode(err) != codes.NotFound {
					return nil, status.Errorf(codes.Internal, "failed to get task status: %v", err)
				}
				// not found, ok to reset
			}
			switch stat.GetState() {
			case task.StateCanceled, task.StateFailed:
				// stale/completed, ok to reset
			case task.StateCompleted:
				// this probably shouldn't happen, but reset anyway to get back to a good state
				return nil, status.Errorf(codes.FailedPrecondition, "uninstall already completed")
			default:
				return nil, status.Errorf(codes.FailedPrecondition, "uninstall is already in progress")
			}
		}
		break
	}
	if !exists {
		return nil, status.Error(codes.FailedPrecondition, "cluster does not have the requested capability")
	}

	now := timestamppb.Now()
	_, err = s.Context.StorageBackend().UpdateCluster(ctx, cluster.Reference(), func(c *corev1.Cluster) {
		for _, cap := range c.Metadata.Capabilities {
			if cap.Name == wellknown.CapabilityMetrics {
				cap.DeletionTimestamp = now
				break
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update cluster metadata: %v", err)
	}
	if err := RequestNodeSync(s.Context, req.Agent); err != nil {
		s.Context.Logger().With(
			logger.Err(err),
			"agent", req.Agent,
		).Warn("sync request failed; agent may not be updated immediately")
		// continue; this is not a fatal error
	}

	md := uninstall.TimestampedMetadata{
		DefaultUninstallOptions: defaultOpts,
		DeletionTimestamp:       now.AsTime(),
	}
	err = s.uninstallController.LaunchTask(req.Agent.Id, task.WithMetadata(md))
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// UninstallStatus implements capabilityv1.BackendServer
func (s *CapabilityBackendService) UninstallStatus(_ context.Context, req *capabilityv1.UninstallStatusRequest) (*corev1.TaskStatus, error) {
	return s.uninstallController.TaskStatus(req.GetAgent().GetId())
}

// CancelUninstall implements capabilityv1.BackendServer
func (s *CapabilityBackendService) CancelUninstall(_ context.Context, req *capabilityv1.CancelUninstallRequest) (*emptypb.Empty, error) {
	s.uninstallController.CancelTask(req.GetAgent().GetId())

	return &emptypb.Empty{}, nil
}

// InstallerTemplate implements capabilityv1.BackendServer
func (s *CapabilityBackendService) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return &capabilityv1.InstallerTemplateResponse{
		Template: `helm install opni-agent ` +
			`{{ arg "input" "Namespace" "+omitEmpty" "+default:opni-agent" "+format:-n {{ value }}" }} ` +
			`oci://docker.io/rancher/opni-agent --version=0.5.4 ` +
			`--set monitoring.enabled=true,token={{ .Token }},pin={{ .Pin }},address={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }} ` +
			`{{ arg "toggle" "Install Prometheus Operator" "+omitEmpty" "+default:false" "+format:--set kube-prometheus-stack.enabled={{ value }}" }} ` +
			`--create-namespace`,
	}, nil
}

// Implements node.NodeMetricsCapabilityServer
func (s *CapabilityBackendService) Sync(ctx context.Context, req *node.SyncRequest) (*node.SyncResponse, error) {
	id := cluster.StreamAuthorizedID(ctx)

	// look up the cluster and check if the capability is installed
	cluster, err := s.Context.StorageBackend().GetCluster(ctx, &corev1.Reference{
		Id: id,
	})
	if err != nil {
		return nil, err
	}
	var enabled bool
	for _, cap := range cluster.GetCapabilities() {
		if cap.Name == wellknown.CapabilityMetrics {
			enabled = (cap.DeletionTimestamp == nil)
		}
	}
	var conditions []string
	if enabled {
		// auto-disable if cortex is not installed
		if err := s.Context.ClusterDriver().ShouldDisableNode(cluster.Reference()); err != nil {
			reason := status.Convert(err).Message()
			s.Context.Logger().With(
				"reason", reason,
			).Info("disabling metrics capability for node")
			enabled = false
			conditions = append(conditions, reason)
		}
	}

	s.nodeStatusMu.Lock()
	defer s.nodeStatusMu.Unlock()

	nodeStatus := s.nodeStatus[id]
	if nodeStatus == nil {
		s.nodeStatus[id] = &capabilityv1.NodeCapabilityStatus{}
		nodeStatus = s.nodeStatus[id]
	}

	nodeStatus.Enabled = req.GetCurrentConfig().GetEnabled()
	nodeStatus.Conditions = req.GetCurrentConfig().GetConditions()
	nodeStatus.LastSync = timestamppb.Now()
	s.Context.Logger().With(
		"id", id,
		"time", nodeStatus.LastSync.AsTime(),
	).Debug("synced node")

	latest, err := s.nodeConfigClient.GetConfiguration(ctx, &node.GetRequest{
		Node: &corev1.Reference{Id: id},
	})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "failed to get node configuration: %v", err)
	}

	if req.GetCurrentConfig().GetSpec().GetRevision().GetRevision() == latest.GetRevision().GetRevision() &&
		req.GetCurrentConfig().GetEnabled() == enabled {
		return &node.SyncResponse{
			ConfigStatus: node.ConfigStatus_UpToDate,
		}, nil
	}
	return &node.SyncResponse{
		ConfigStatus: node.ConfigStatus_NeedsUpdate,
		UpdatedConfig: &node.MetricsCapabilityStatus{
			Enabled:    enabled,
			Conditions: conditions,
			Spec:       latest,
		},
	}, nil
}

type UninstallTaskRunner struct {
	uninstall.DefaultPendingHandler

	sc              types.ServiceContext
	cortexClientSet *memoize.Promise
}

func NewUninstallTaskRunner(ctx types.ServiceContext) (*UninstallTaskRunner, error) {
	return &UninstallTaskRunner{
		sc:              ctx,
		cortexClientSet: ctx.Memoize(cortex.NewClientSet(ctx.GatewayConfig())),
	}, nil
}

func (a *UninstallTaskRunner) OnTaskRunning(ctx context.Context, ti task.ActiveTask) error {
	var md uninstall.TimestampedMetadata
	ti.LoadTaskMetadata(&md)

	ti.AddLogEntry(slog.LevelInfo, "Uninstalling metrics capability for this cluster")

	if md.DeleteStoredData {
		ti.AddLogEntry(slog.LevelWarn, "Will delete time series data")
		if err := a.deleteTenant(ctx, ti.TaskId()); err != nil {
			return err
		}
		ti.AddLogEntry(slog.LevelInfo, "Delete request accepted; polling status")

		p := backoff.Exponential(
			backoff.WithMaxRetries(0),
			backoff.WithMinInterval(5*time.Second),
			backoff.WithMaxInterval(1*time.Minute),
			backoff.WithMultiplier(1.1),
		)
		b := p.Start(ctx)
	RETRY:
		for {
			select {
			case <-b.Done():
				ti.AddLogEntry(slog.LevelWarn, "Uninstall canceled, but time series data is still being deleted by Cortex")
				return ctx.Err()
			case <-b.Next():
				status, err := a.tenantDeleteStatus(ctx, ti.TaskId())
				if err != nil {
					continue
				}
				if status.BlocksDeleted {
					ti.AddLogEntry(slog.LevelInfo, "Time series data deleted successfully")
					break RETRY
				}
			}
		}
	} else {
		ti.AddLogEntry(slog.LevelInfo, "Time series data will not be deleted")
	}

	ti.AddLogEntry(slog.LevelInfo, "Removing capability from cluster metadata")
	_, err := a.sc.StorageBackend().UpdateCluster(ctx, &corev1.Reference{
		Id: ti.TaskId(),
	}, storage.NewRemoveCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityMetrics)))
	if err != nil {
		return err
	}
	return nil
}

func (a *UninstallTaskRunner) OnTaskCompleted(ctx context.Context, ti task.ActiveTask, state task.State, args ...any) {
	switch state {
	case task.StateCompleted:
		ti.AddLogEntry(slog.LevelInfo, "Capability uninstalled successfully")
		return // no deletion timestamp to reset, since the capability should be gone
	case task.StateFailed:
		ti.AddLogEntry(slog.LevelError, fmt.Sprintf("Capability uninstall failed: %v", args[0]))
	case task.StateCanceled:
		ti.AddLogEntry(slog.LevelInfo, "Capability uninstall canceled")
	}

	// Reset the deletion timestamp
	_, err := a.sc.StorageBackend().UpdateCluster(ctx, &corev1.Reference{
		Id: ti.TaskId(),
	}, func(c *corev1.Cluster) {
		for _, cap := range c.GetCapabilities() {
			if cap.Name == wellknown.CapabilityMetrics {
				cap.DeletionTimestamp = nil
			}
		}
	})
	if err != nil {
		ti.AddLogEntry(slog.LevelWarn, fmt.Sprintf("Failed to reset deletion timestamp: %v", err))
	}
}

func (a *UninstallTaskRunner) deleteTenant(ctx context.Context, clusterId string) error {
	endpoint := fmt.Sprintf("https://%s/purger/delete_tenant", a.sc.GatewayConfig().Spec.Cortex.Purger.HTTPAddress)
	deleteReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	deleteReq.Header.Set(cortex.OrgIDCodec.Key(), cortex.OrgIDCodec.Encode([]string{clusterId}))
	cs, err := cortex.AcquireClientSet(ctx, a.cortexClientSet)
	if err != nil {
		return err
	}
	resp, err := cs.HTTP().Do(deleteReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusInternalServerError:
		msg, _ := io.ReadAll(resp.Body)
		return status.Error(codes.Internal, fmt.Sprintf("cortex internal server error: %s", string(msg)))
	default:
		msg, _ := io.ReadAll(resp.Body)
		return status.Error(codes.Internal, fmt.Sprintf("unexpected response from cortex: %s", string(msg)))
	}
}

func (a *UninstallTaskRunner) tenantDeleteStatus(ctx context.Context, clusterId string) (*purger.DeleteTenantStatusResponse, error) {
	endpoint := fmt.Sprintf("https://%s/purger/delete_tenant_status", a.sc.GatewayConfig().Spec.Cortex.Purger.HTTPAddress)

	statusReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	statusReq.Header.Set(cortex.OrgIDCodec.Key(), cortex.OrgIDCodec.Encode([]string{clusterId}))
	cs, err := cortex.AcquireClientSet(ctx, a.cortexClientSet)
	if err != nil {
		return nil, err
	}
	resp, err := cs.HTTP().Do(statusReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var status purger.DeleteTenantStatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			return nil, err
		}
		return &status, nil
	default:
		msg, _ := io.ReadAll(resp.Body)
		return nil, status.Error(codes.Internal, fmt.Sprintf("cortex internal server error: %s", string(msg)))
	}
}

func init() {
	types.Services.Register("Capability Backend Service", func(_ context.Context, opts ...driverutil.Option) (types.Service, error) {
		svc := &CapabilityBackendService{
			nodeStatus: make(map[string]*capabilityv1.NodeCapabilityStatus),
		}
		driverutil.ApplyOptions(svc, opts...)
		return svc, nil
	})
}
