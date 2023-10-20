package backend

import (
	"context"
	"fmt"
	"sync"
	"time"

	"log/slog"

	"github.com/google/go-cmp/cmp"
	"github.com/rancher/opni/pkg/agent"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/topology/apis/node"
	"github.com/rancher/opni/plugins/topology/apis/orchestrator"
	"github.com/rancher/opni/plugins/topology/pkg/topology/gateway/drivers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type TopologyBackendConfig struct {
	Logger              *slog.Logger                              `validate:"required"`
	StorageBackend      storage.Backend                           `validate:"required"`
	MgmtClient          managementv1.ManagementClient             `validate:"required"`
	Delegate            streamext.StreamDelegate[agent.ClientSet] `validate:"required"`
	UninstallController *task.Controller                          `validate:"required"`
	ClusterDriver       drivers.ClusterDriver                     `validate:"required"`
}

type TopologyBackend struct {
	capabilityv1.UnsafeBackendServer
	node.UnsafeNodeTopologyCapabilityServer
	orchestrator.UnsafeTopologyOrchestratorServer
	TopologyBackendConfig

	nodeStatusMu sync.RWMutex
	nodeStatus   map[string]*capabilityv1.NodeCapabilityStatus

	desiredNodeSpecMu sync.RWMutex
	desiredNodeSpec   map[string]*node.TopologyCapabilitySpec

	util.Initializer
}

func (t *TopologyBackend) GetClusterStatus(_ context.Context, _ *emptypb.Empty) (*orchestrator.InstallStatus, error) {
	// nothing to install
	return &orchestrator.InstallStatus{
		State: orchestrator.InstallState_Installed,
	}, nil
}

var _ node.NodeTopologyCapabilityServer = (*TopologyBackend)(nil)
var _ capabilityv1.BackendServer = (*TopologyBackend)(nil)
var _ orchestrator.TopologyOrchestratorServer = (*TopologyBackend)(nil)

func (t *TopologyBackend) Initialize(conf TopologyBackendConfig) {
	t.InitOnce(func() {
		t.TopologyBackendConfig = conf
		t.nodeStatus = make(map[string]*capabilityv1.NodeCapabilityStatus)
		t.desiredNodeSpec = make(map[string]*node.TopologyCapabilitySpec)
	})
}

func (t *TopologyBackend) Info(_ context.Context, _ *emptypb.Empty) (*capabilityv1.Details, error) {
	// !! Info must never block
	var drivers []string
	if t.Initialized() {
		drivers = append(drivers, t.ClusterDriver.Name())
	}

	return &capabilityv1.Details{
		Name:    wellknown.CapabilityTopology,
		Source:  "plugin_topology",
		Drivers: drivers,
	}, nil
}

func (t *TopologyBackend) CanInstall(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	t.WaitForInit()
	return &emptypb.Empty{}, nil
}

func (t *TopologyBackend) canInstall(ctx context.Context) error {
	stat, err := t.ClusterDriver.GetClusterStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return status.Error(codes.Unavailable, err.Error())
	}
	switch stat.GetState() {
	case orchestrator.InstallState_Updating, orchestrator.InstallState_Installed:
		// ok
	case orchestrator.InstallState_NotInstalled, orchestrator.InstallState_Uninstalling:
		return status.Error(codes.Unavailable, "topology cluster is not installed")
	case orchestrator.InstallState_Unknown:
		fallthrough
	default:
		return status.Error(codes.Internal, "unkown topology cluster state")
	}
	return nil
}

func (t *TopologyBackend) requestNodeSync(ctx context.Context, cluster *corev1.Reference) {
	_, err := t.Delegate.WithTarget(cluster).SyncNow(ctx, &capabilityv1.Filter{
		CapabilityNames: []string{wellknown.CapabilityTopology},
	})
	name := cluster.GetId()
	if name == "" {
		name = "(all)"
	}
	if err != nil {
		t.Logger.With(
			"cluster", name,
			"capability", wellknown.CapabilityTopology,
			logger.Err(err),
		).Warn("failed to request node sync; nodes may not be updated immediately")
		return
	}
	t.Logger.With(
		"cluster", name,
		"capability", wellknown.CapabilityTopology,
	).Info("node sync requested")
}

func (t *TopologyBackend) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	ctxTimeout, ca := context.WithTimeout(ctx, time.Second*60)
	defer ca()
	if err := t.WaitForInitContext(ctxTimeout); err != nil {
		// !! t.logger is not initialized if the deadline is exceeded
		return nil, err
	}

	var warningErr error
	if err := t.canInstall(ctx); err != nil {
		if !req.IgnoreWarnings {
			return &capabilityv1.InstallResponse{
				Status:  capabilityv1.InstallResponseStatus_Error,
				Message: err.Error(),
			}, nil
		}
		warningErr = err
	}

	_, err := t.StorageBackend.UpdateCluster(ctx, req.Cluster,
		storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityTopology)),
	)
	if err != nil {
		return nil, err
	}

	t.requestNodeSync(ctx, req.Cluster)

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

func (t *TopologyBackend) Status(_ context.Context, req *corev1.Reference) (*capabilityv1.NodeCapabilityStatus, error) {
	t.WaitForInit()

	t.nodeStatusMu.RLock()
	defer t.nodeStatusMu.RUnlock()

	if status, ok := t.nodeStatus[req.Id]; ok {
		return status, nil
	}

	return nil, status.Error(codes.NotFound, "no status has been reported for this node")
}

func (t *TopologyBackend) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	t.WaitForInit()

	cluster, err := t.MgmtClient.GetCluster(ctx, req.Cluster)
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
		if cap.Name != wellknown.CapabilityTopology {
			continue
		}
		exists = true
		if cap.DeletionTimestamp != nil {
			stat, err := t.UninstallController.TaskStatus(cluster.Id)
			if err != nil {
				if util.StatusCode(err) != codes.NotFound {
					return nil, status.Errorf(codes.Internal, "failed to get task status : %v", err)
				}
			}

			switch stat.GetState() {
			case task.StateCanceled, task.StateFailed:
				// ok to reset
			case task.StateCompleted:
				return nil, status.Errorf(codes.FailedPrecondition, "uninstall already completed")
			default:
				return nil, status.Errorf(codes.FailedPrecondition, "uninstall already is already in progress")
			}
		}
		break
	}
	if !exists {
		return nil, status.Error(codes.FailedPrecondition, "cluster does not have the requested capability")
	}
	now := timestamppb.Now()
	_, updateErr := t.StorageBackend.UpdateCluster(ctx, cluster.Reference(), func(c *corev1.Cluster) {
		for _, cap := range c.Metadata.Capabilities {
			if cap.Name == wellknown.CapabilityTopology {
				cap.DeletionTimestamp = now
				break
			}
		}
	})
	if updateErr != nil {
		return nil, fmt.Errorf("failed to update cluster metadata : %v", err)
	}
	t.requestNodeSync(ctx, req.Cluster)

	md := uninstall.TimestampedMetadata{
		DefaultUninstallOptions: defaultOpts,
		DeletionTimestamp:       now.AsTime(),
	}
	err = t.UninstallController.LaunchTask(req.Cluster.Id, task.WithMetadata(md))
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (t *TopologyBackend) UninstallStatus(_ context.Context, cluster *corev1.Reference) (*corev1.TaskStatus, error) {
	t.WaitForInit()

	return t.UninstallController.TaskStatus(cluster.Id)
}

func (t *TopologyBackend) CancelUninstall(ctx context.Context, cluster *corev1.Reference) (*emptypb.Empty, error) {
	t.WaitForInit()

	t.UninstallController.CancelTask(cluster.Id)
	t.requestNodeSync(ctx, cluster)
	return &emptypb.Empty{}, nil
}

// ! depecrated : agentv1 only
func (t *TopologyBackend) InstallerTemplate(_ context.Context, _ *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	t.WaitForInit()

	return nil, status.Error(codes.Unimplemented, "method not implemented : topology does not have an agentv1 implementation")
}

func (t *TopologyBackend) Sync(ctx context.Context, req *node.SyncRequest) (*node.SyncResponse, error) {
	t.WaitForInit()

	id := cluster.StreamAuthorizedID(ctx)

	// look up the cluster and check if the capability is installed
	cluster, err := t.StorageBackend.GetCluster(ctx, &corev1.Reference{
		Id: id,
	})
	if err != nil {
		return nil, err
	}
	var enabled bool
	for _, cap := range cluster.GetCapabilities() {
		if cap.Name == wellknown.CapabilityTopology {
			enabled = (cap.DeletionTimestamp == nil)
		}
	}
	var conditions []string
	if enabled {
		if err := t.ClusterDriver.ShouldDisableNode(cluster.Reference()); err != nil {
			reason := status.Convert(err).Message()
			t.Logger.With(
				"reason", reason,
			)
		}
	}

	t.desiredNodeSpecMu.Lock()
	defer t.desiredNodeSpecMu.Unlock()

	t.nodeStatusMu.Lock()
	defer t.nodeStatusMu.Unlock()

	status := t.nodeStatus[id]
	if status == nil {
		t.Logger.Debug("No current status found, setting to default")
		t.nodeStatus[id] = &capabilityv1.NodeCapabilityStatus{}
		status = t.nodeStatus[id]
	}
	status.Enabled = req.GetCurrentConfig().GetEnabled()
	status.LastSync = timestamppb.Now()

	//  TODO(topology) : allow for additional customization
	return buildResponse(req.GetCurrentConfig(), &node.TopologyCapabilityConfig{
		Enabled:    enabled,
		Conditions: conditions,
		Spec:       &node.TopologyCapabilitySpec{
			// TODO(topology)
		},
	}), nil
}

// !! the calling function must have exclusive ownership of both old and new
func buildResponse(old, new *node.TopologyCapabilityConfig) *node.SyncResponse {
	if cmp.Equal(old, new, protocmp.Transform()) {
		return &node.SyncResponse{
			ConfigStatus: node.ConfigStatus_UpToDate,
		}
	}
	return &node.SyncResponse{
		ConfigStatus:  node.ConfigStatus_NeedsUpdate,
		UpdatedConfig: new,
	}
}
