package backend

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/proto"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/logging/pkg/apis/node"
	loggingerrors "github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (b *LoggingBackend) Status(ctx context.Context, req *capabilityv1.StatusRequest) (*capabilityv1.NodeCapabilityStatus, error) {
	b.WaitForInit()

	b.nodeStatusMu.RLock()
	defer b.nodeStatusMu.RUnlock()

	capStatus, err := b.ClusterDriver.GetClusterStatus(ctx, req.GetCluster().GetId())
	if err != nil {
		if errors.Is(err, loggingerrors.ErrInvalidList) {
			return nil, status.Error(codes.NotFound, "unable to list cluster status")
		}
		return nil, err
	}

	return capStatus, nil
}

func (b *LoggingBackend) Sync(ctx context.Context, req *node.SyncRequest) (*node.SyncResponse, error) {
	b.WaitForInit()

	id, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
	if !ok {
		return nil, util.StatusError(codes.Unauthenticated)
	}

	// look up the cluster and check if the capability is installed
	cluster, err := b.StorageBackend.GetCluster(ctx, &opnicorev1.Reference{
		Id: id,
	})
	if err != nil {
		return nil, err
	}

	var enabled bool
	for _, cap := range cluster.GetCapabilities() {
		if cap.Name == wellknown.CapabilityLogs {
			enabled = (cap.DeletionTimestamp == nil)
		}
	}
	var conditions []string
	if enabled {
		if b.shouldDisableNode(ctx) {
			reason := "opensearch is not installed"
			enabled = false
			conditions = append(conditions, reason)
		}
	}

	b.desiredNodeSpecMu.RLock()
	defer b.desiredNodeSpecMu.RUnlock()

	b.nodeStatusMu.RLock()
	defer b.nodeStatusMu.RUnlock()

	err = b.ClusterDriver.SetClusterStatus(ctx, id, enabled)
	if err != nil {
		return nil, err
	}

	osConf, err := b.getOpensearchConfig(ctx, id)
	if err != nil {
		return nil, err
	}

	return b.buildResponse(req.CurrentConfig, &node.LoggingCapabilityConfig{
		Enabled:          enabled,
		Conditions:       conditions,
		OpensearchConfig: osConf,
	}), nil
}

func (b *LoggingBackend) buildResponse(old, new *node.LoggingCapabilityConfig) *node.SyncResponse {
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

func (b *LoggingBackend) getOpensearchConfig(ctx context.Context, id string) (*node.OpensearchConfig, error) {
	username, password := b.ClusterDriver.GetCredentials(ctx, id)

	return &node.OpensearchConfig{
		Username:       username,
		Password:       password,
		Url:            b.ClusterDriver.GetExternalURL(ctx),
		TracingEnabled: true,
	}, nil
}

func (b *LoggingBackend) shouldDisableNode(ctx context.Context) bool {
	installState := b.ClusterDriver.GetInstallStatus(ctx)
	switch installState {
	case drivers.Absent:
		return true
	case drivers.Pending, drivers.Installed:
		return false
	case drivers.Error:
		fallthrough
	default:
		// if status is unknown don't uninstall from the node
	}
	return false
}

func (b *LoggingBackend) requestNodeSync(ctx context.Context, cluster *opnicorev1.Reference) {
	_, err := b.NodeManagerClient.RequestSync(ctx, &capabilityv1.SyncRequest{
		Cluster: cluster,
		Filter: &capabilityv1.Filter{
			CapabilityNames: []string{wellknown.CapabilityLogs},
		},
	})

	name := cluster.GetId()
	if name == "" {
		name = "(all)"
	}
	if err != nil {
		b.Logger.With(
			"cluster", name,
			"capability", wellknown.CapabilityLogs,
			zap.Error(err),
		).Warn("failed to request node sync; nodes may not be updated immediately")
		return
	}
	b.Logger.With(
		"cluster", name,
		"capability", wellknown.CapabilityLogs,
	).Info("node sync requested")
}
