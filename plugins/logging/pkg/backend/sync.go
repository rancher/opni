package backend

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/logging/apis/node"
	loggingerrors "github.com/rancher/opni/plugins/logging/pkg/errors"
	driver "github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
)

func (b *LoggingBackend) Status(ctx context.Context, req *opnicorev1.Reference) (*capabilityv1.NodeCapabilityStatus, error) {
	b.WaitForInit()

	b.nodeStatusMu.RLock()
	defer b.nodeStatusMu.RUnlock()

	capStatus, err := b.ClusterDriver.GetClusterStatus(ctx, req.GetId())
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

	id := cluster.StreamAuthorizedID(ctx)

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

	if enabled {
		err = b.ClusterDriver.SetClusterStatus(ctx, id, enabled)
		if err != nil {
			return nil, err
		}
	} else {
		b.ClusterDriver.SetSyncTime()
	}

	return b.buildResponse(req.GetCurrentConfig(), &node.LoggingCapabilityConfig{
		Enabled:    enabled,
		Conditions: conditions,
	}), nil
}

func (b *LoggingBackend) buildResponse(old, new *node.LoggingCapabilityConfig) *node.SyncResponse {
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

func (b *LoggingBackend) shouldDisableNode(ctx context.Context) bool {
	installState := b.ClusterDriver.GetInstallStatus(ctx)
	switch installState {
	case driver.Absent:
		return true
	case driver.Pending, driver.Installed:
		return false
	case driver.Error:
		fallthrough
	default:
		// if status is unknown don't uninstall from the node
	}
	return false
}

func (b *LoggingBackend) requestNodeSync(ctx context.Context, cluster *opnicorev1.Reference) {
	_, err := b.Delegate.WithTarget(cluster).SyncNow(ctx, &capabilityv1.Filter{
		CapabilityNames: []string{wellknown.CapabilityLogs},
	})

	name := cluster.GetId()
	if name == "" {
		name = "(all)"
	}
	if err != nil {
		b.Logger.With(
			"cluster", name,
			"capability", wellknown.CapabilityLogs,
			logger.Err(err),
		).Warn("failed to request node sync; nodes may not be updated immediately")
		return
	}
	b.Logger.With(
		"cluster", name,
		"capability", wellknown.CapabilityLogs,
	).Info("node sync requested")
}
