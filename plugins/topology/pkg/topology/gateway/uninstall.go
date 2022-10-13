package gateway

import (
	"context"
	"fmt"

	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

type TopologyUninstallTaskRunner struct {
	logger         *zap.SugaredLogger
	storageBackend storage.Backend
}

func (n *TopologyUninstallTaskRunner) OnTaskPending(ctx context.Context, ti task.ActiveTask) error {
	return nil
}

func (n *TopologyUninstallTaskRunner) OnTaskRunning(ctx context.Context, ti task.ActiveTask) error {
	var md uninstall.TimestampedMetadata
	ti.LoadTaskMetadata(&md)
	ti.AddLogEntry(zapcore.InfoLevel, "Uninstalling topology capability for this cluster")
	_, err := n.storageBackend.UpdateCluster(ctx, &corev1.Reference{
		Id: ti.TaskId(),
	}, storage.NewRemoveCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityTopology)))
	if err != nil {
		ti.AddLogEntry(zapcore.ErrorLevel, fmt.Sprintf("failed to remove topology capability from cluster : %s", err))
		return err
	}
	return nil
}

func (n *TopologyUninstallTaskRunner) OnTaskCompleted(ctx context.Context, ti task.ActiveTask, state task.State, args ...any) {
	// noop

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
	_, err := n.storageBackend.UpdateCluster(ctx, &corev1.Reference{
		Id: ti.TaskId(),
	}, func(c *corev1.Cluster) {
		for _, cap := range c.GetCapabilities() {
			if cap.Name == wellknown.CapabilityTopology {
				cap.DeletionTimestamp = nil
			}
		}
	})
	if err != nil {
		ti.AddLogEntry(zapcore.WarnLevel, fmt.Sprintf("Failed to reset deletion timestamp: %v", err))
	}
}

var _ task.TaskRunner = (*TopologyUninstallTaskRunner)(nil)
