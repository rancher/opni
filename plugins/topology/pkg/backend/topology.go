package backend

import (
	"context"
	"sync"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/topology/pkg/apis/node"
	"github.com/rancher/opni/plugins/topology/pkg/apis/orchestrator"
	"github.com/rancher/opni/plugins/topology/pkg/topology/gateway/drivers"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type TopologyBackendConfig struct {
	Logger              *zap.SugaredLogger             `validate:"required"`
	StorageBackend      storage.Backend                `validate:"required"`
	MgmtClient          managementv1.ManagementClient  `validate:"required"`
	NodeManagerClient   capabilityv1.NodeManagerClient `validate:"required"`
	UninstallController *task.Controller               `validate:"required"`
	ClusterDriver       drivers.ClusterDriver          `validate:"required"`
}

type TopologyBackend struct {
	capabilityv1.UnsafeBackendServer
	node.UnsafeNodeTopologyCapabilityServer
	TopologyBackendConfig

	nodeStatusMu sync.RWMutex
	nodeStatus   map[string]*capabilityv1.NodeCapabilityStatus

	desiredNodeSpecMu sync.RWMutex
	desiredNodeSpec   map[string]*node.TopologyCapabilitySpec

	util.Initializer
}

var _ node.NodeTopologyCapabilityServer = (*TopologyBackend)(nil)
var _ capabilityv1.BackendServer = (*TopologyBackend)(nil)

func (t *TopologyBackend) Initialize(conf TopologyBackendConfig) {
	t.InitOnce(func() {
		// TODO : initialization code goes here
	})
}

func (t *TopologyBackend) Info(_ context.Context, _ *emptypb.Empty) (*capabilityv1.InfoResponse, error) {
	// !! Info must never block
	return &capabilityv1.InfoResponse{
		CapabilityName: wellknown.CapabilityTopology,
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
	_, err := t.NodeManagerClient.RequestSync(ctx, &capabilityv1.SyncRequest{
		Cluster: cluster,
		Filter: &capabilityv1.Filter{
			CapabilityNames: []string{wellknown.CapabilityTopology},
		},
	})
	name := cluster.GetId()
	if name == "" {
		name = "(all)"
	}
	if err != nil {
		t.Logger.With(
			"cluster", name,
			"capability", wellknown.CapabilityTopology,
			zap.Error(err),
		).Warn("failed to request node sync; nodes may not be updated immediately")
		return
	}
	t.Logger.With(
		"cluster", name,
		"capability", wellknown.CapabilityTopology,
	).Info("node sync requested")
}

func (t *TopologyBackend) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	t.WaitForInit()

	var warningErr error
	if err := t.canInstall(ctx); err != nil {
		if !req.IgnoreWarnings {
			return &capabilityv1.InstallResponse{
				Status:  capabilityv1.InstallResponseStatus_Error,
				Message: err.Error(),
			}, nil
		} else {
			warningErr = err
		}
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

func (t *TopologyBackend) Status(ctx context.Context, req *capabilityv1.StatusRequest) (*capabilityv1.NodeCapabilityStatus, error) {
	t.WaitForInit()

	t.nodeStatusMu.RLock()
	defer t.nodeStatusMu.RUnlock()

	if status, ok := t.nodeStatus[req.Cluster.Id]; ok {
		return status, nil
	}

	return nil, status.Error(codes.NotFound, "no status has been reported for this node")
}

func (t *TopologyBackend) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	t.WaitForInit()

	// TODO : implement me if necessary

	return &emptypb.Empty{}, nil
}

func (t *TopologyBackend) UninstallStatus(ctx context.Context, reference *corev1.Reference) (*corev1.TaskStatus, error) {
	t.WaitForInit()

	// TODO : implement me if necessary
	return &corev1.TaskStatus{
		State: corev1.TaskState_Completed,
	}, nil
}

func (t *TopologyBackend) CancelUninstall(ctx context.Context, reference *corev1.Reference) (*emptypb.Empty, error) {
	t.WaitForInit()
	//TODO implement me if necessary

	return &emptypb.Empty{}, nil
}

// ! depecrated : agentv1 only
func (t *TopologyBackend) InstallerTemplate(ctx context.Context, empty *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	t.WaitForInit()

	return nil, status.Error(codes.Unimplemented, "method not implemented : topology does not have an agentv1 implementation")
}

func (t *TopologyBackend) Sync(ctx context.Context, req *node.SyncRequest) (*node.SyncResponse, error) {
	t.WaitForInit()

	id, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
	if !ok {
		return nil, util.StatusError(codes.Unauthenticated)
	}

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

	t.desiredNodeSpecMu.RLock()
	defer t.desiredNodeSpecMu.RUnlock()

	t.nodeStatusMu.RLock()
	defer t.nodeStatusMu.RUnlock()

	status := t.nodeStatus[id]
	if status == nil {
		t.nodeStatus[id] = &capabilityv1.NodeCapabilityStatus{}
		status = t.nodeStatus[id]
	}
	status.Enabled = req.GetCurrentConfig().GetEnabled()
	status.LastSync = timestamppb.Now()

	//  TODO : allow for additional customization
	return buildResponse(req.GetCurrentConfig(), &node.TopologyCapabilityConfig{
		Enabled:    enabled,
		Conditions: conditions,
		Spec:       &node.TopologyCapabilitySpec{
			// TODO
		},
	}), nil
}

// !! the calling function must have exclusive ownership of both old and new
func buildResponse(old, new *node.TopologyCapabilityConfig) *node.SyncResponse {
	if proto.Equal(old, new) {
		return &node.SyncResponse{
			ConfigStatus: node.ConfigStatus_UpToDate,
		}
	}
	return &node.SyncResponse{
		ConfigStatus:  node.ConfigStatus_NeedsUpdate,
		UpdatedConfig: new,
	}
}
