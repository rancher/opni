package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/cortexproject/cortex/pkg/purger"
	"github.com/lestrrat-go/backoff/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UninstallTaskRunner struct {
	uninstall.DefaultPendingHandler
	UninstallTaskRunnerConfig

	util.Initializer
}

type UninstallTaskRunnerConfig struct {
	CortexClientSet ClientSet                  `validate:"required"`
	Config          *v1beta1.GatewayConfigSpec `validate:"required"`
	StorageBackend  storage.Backend            `validate:"required"`
}

func (a *UninstallTaskRunner) Initialize(conf UninstallTaskRunnerConfig) {
	a.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		a.UninstallTaskRunnerConfig = conf
	})
}

func (a *UninstallTaskRunner) OnTaskRunning(ctx context.Context, ti task.ActiveTask) error {
	a.WaitForInit()

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
	_, err := a.StorageBackend.UpdateCluster(ctx, &corev1.Reference{
		Id: ti.TaskId(),
	}, storage.NewRemoveCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityMetrics)))
	if err != nil {
		return err
	}
	return nil
}

func (a *UninstallTaskRunner) OnTaskCompleted(ctx context.Context, ti task.ActiveTask, state task.State, args ...any) {
	a.WaitForInit()

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
	_, err := a.StorageBackend.UpdateCluster(ctx, &corev1.Reference{
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
	endpoint := fmt.Sprintf("https://%s/purger/delete_tenant", a.Config.Cortex.Purger.HTTPAddress)
	deleteReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	deleteReq.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{clusterId}))
	resp, err := a.CortexClientSet.HTTP().Do(deleteReq)
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
	endpoint := fmt.Sprintf("https://%s/purger/delete_tenant_status", a.Config.Cortex.Purger.HTTPAddress)

	statusReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	statusReq.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{clusterId}))
	resp, err := a.CortexClientSet.HTTP().Do(statusReq)
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
