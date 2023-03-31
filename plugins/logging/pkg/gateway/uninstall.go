package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util/future"
	backenddriver "github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend"
	"github.com/rancher/opni/plugins/logging/pkg/opensearchdata"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type UninstallTaskRunner struct {
	uninstall.DefaultPendingHandler
	storageNamespace  string
	opensearchManager *opensearchdata.Manager
	backendDriver     backenddriver.ClusterDriver
	storageBackend    future.Future[storage.Backend]
	logger            *zap.SugaredLogger
}

func (a *UninstallTaskRunner) OnTaskRunning(ctx context.Context, ti task.ActiveTask) error {
	var md uninstall.TimestampedMetadata
	ti.LoadTaskMetadata(&md)

	ti.AddLogEntry(zapcore.InfoLevel, "Uninstalling logging capability for this cluster")

	if md.DeleteStoredData {
		ti.AddLogEntry(zapcore.WarnLevel, "Will delete opensearch data")
		if err := a.opensearchManager.DoClusterDataDelete(ctx, ti.TaskId()); err != nil {
			ti.AddLogEntry(zapcore.ErrorLevel, err.Error())
			return err
		}

		ti.AddLogEntry(zapcore.InfoLevel, "Delete request accepted; polling status")

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
				ti.AddLogEntry(zapcore.WarnLevel, "Uninstall canceled, logging data is still being deleted")
				return ctx.Err()
			case <-b.Next():
				status, err := a.opensearchManager.DeleteTaskStatus(ctx, ti.TaskId())
				if err != nil {
					ti.AddLogEntry(zapcore.ErrorLevel, err.Error())
					continue
				}
				switch status {
				case opensearchdata.DeletePending:
					if err := a.opensearchManager.DoClusterDataDelete(ctx, ti.TaskId()); err != nil {
						ti.AddLogEntry(zapcore.ErrorLevel, err.Error())
						continue
					}
				case opensearchdata.DeleteFinishedWithErrors:
					ti.AddLogEntry(zapcore.WarnLevel, "some log entries not deleted")
					break RETRY
				case opensearchdata.DeleteFinished:
					ti.AddLogEntry(zapcore.InfoLevel, "Logging data deleted successfully")
					break RETRY
				}
			}
		}
	} else {
		ti.AddLogEntry(zapcore.InfoLevel, "Log data will not be deleted")
	}

	ti.AddLogEntry(zapcore.InfoLevel, "Deleting Kubernetes data")
	err := a.backendDriver.DeleteCluster(ctx, ti.TaskId())
	if err != nil {
		ti.AddLogEntry(zapcore.ErrorLevel, err.Error())
		return err
	}

	ti.AddLogEntry(zapcore.InfoLevel, "Removing capability from cluster metadata")
	_, err = a.storageBackend.Get().UpdateCluster(ctx, &opnicorev1.Reference{
		Id: ti.TaskId(),
	}, storage.NewRemoveCapabilityMutator[*opnicorev1.Cluster](capabilities.Cluster(wellknown.CapabilityLogs)))
	if err != nil {
		return err
	}
	return nil
}

func (a *UninstallTaskRunner) OnTaskCompleted(ctx context.Context, ti task.ActiveTask, state task.State, args ...any) {
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
	_, err := a.storageBackend.Get().UpdateCluster(ctx, &opnicorev1.Reference{
		Id: ti.TaskId(),
	}, func(c *opnicorev1.Cluster) {
		for _, cap := range c.GetCapabilities() {
			if cap.Name == wellknown.CapabilityLogs {
				cap.DeletionTimestamp = nil
			}
		}
	})
	if err != nil {
		ti.AddLogEntry(zapcore.WarnLevel, fmt.Sprintf("Failed to reset deletion timestamp: %v", err))
	}
}
