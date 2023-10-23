package backend

import (
	"context"
	"fmt"

	v1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (m *MetricsBackend) Info(_ context.Context, _ *emptypb.Empty) (*v1.Details, error) {
	// Info must not block
	return &v1.Details{
		Name:    wellknown.CapabilityMetrics,
		Source:  "plugin_metrics",
		Drivers: drivers.ClusterDrivers.List(),
	}, nil
}

func (m *MetricsBackend) CanInstall(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	m.WaitForInit()

	return &emptypb.Empty{}, nil
}

func (m *MetricsBackend) canInstall(ctx context.Context) error {
	stat, err := m.ClusterDriver.Status(ctx, &emptypb.Empty{})
	if err != nil {
		return status.Error(codes.Unavailable, err.Error())
	}
	switch stat.InstallState {
	case driverutil.InstallState_Installed:
		// ok
	case driverutil.InstallState_NotInstalled, driverutil.InstallState_Uninstalling:
		return status.Error(codes.Unavailable, fmt.Sprintf("cortex cluster is not installed"))
	default:
		return status.Error(codes.Internal, fmt.Sprintf("unknown cortex cluster state"))
	}
	return nil
}

func (m *MetricsBackend) Install(ctx context.Context, req *v1.InstallRequest) (*v1.InstallResponse, error) {
	m.WaitForInit()

	var warningErr error
	err := m.canInstall(ctx)
	if err != nil {
		if !req.IgnoreWarnings {
			return &v1.InstallResponse{
				Status:  v1.InstallResponseStatus_Error,
				Message: err.Error(),
			}, nil
		}
		warningErr = err
	}

	_, err = m.StorageBackend.UpdateCluster(ctx, req.Cluster,
		storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityMetrics)),
	)
	if err != nil {
		return nil, err
	}

	if err := m.requestNodeSync(ctx, req.Cluster); err != nil {
		return &v1.InstallResponse{
			Status:  v1.InstallResponseStatus_Warning,
			Message: fmt.Errorf("sync request failed; agent may not be updated immediately: %v", err).Error(),
		}, nil
	}

	if warningErr != nil {
		return &v1.InstallResponse{
			Status:  v1.InstallResponseStatus_Warning,
			Message: warningErr.Error(),
		}, nil
	}
	return &v1.InstallResponse{
		Status: v1.InstallResponseStatus_Success,
	}, nil
}

func (m *MetricsBackend) Status(_ context.Context, req *corev1.Reference) (*v1.NodeCapabilityStatus, error) {
	m.WaitForInit()

	m.nodeStatusMu.RLock()
	defer m.nodeStatusMu.RUnlock()

	if status, ok := m.nodeStatus[req.Id]; ok {
		return util.ProtoClone(status), nil
	}

	return nil, status.Error(codes.NotFound, "no status has been reported for this node")
}

func (m *MetricsBackend) Uninstall(ctx context.Context, req *v1.UninstallRequest) (*emptypb.Empty, error) {
	m.WaitForInit()

	cluster, err := m.MgmtClient.GetCluster(ctx, req.Cluster)
	if err != nil {
		return nil, err
	}

	var defaultOpts v1.DefaultUninstallOptions
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
			stat, err := m.UninstallController.TaskStatus(cluster.Id)
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
	_, err = m.StorageBackend.UpdateCluster(ctx, cluster.Reference(), func(c *corev1.Cluster) {
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
	if err := m.requestNodeSync(ctx, req.Cluster); err != nil {
		m.Logger.With(
			logger.Err(err),
			"agent", req.Cluster,
		).Warn("sync request failed; agent may not be updated immediately")
		// continue; this is not a fatal error
	}

	md := uninstall.TimestampedMetadata{
		DefaultUninstallOptions: defaultOpts,
		DeletionTimestamp:       now.AsTime(),
	}
	err = m.UninstallController.LaunchTask(req.Cluster.Id, task.WithMetadata(md))
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (m *MetricsBackend) UninstallStatus(_ context.Context, cluster *corev1.Reference) (*corev1.TaskStatus, error) {
	m.WaitForInit()

	return m.UninstallController.TaskStatus(cluster.Id)
}

func (m *MetricsBackend) CancelUninstall(_ context.Context, cluster *corev1.Reference) (*emptypb.Empty, error) {
	m.WaitForInit()

	m.UninstallController.CancelTask(cluster.Id)

	return &emptypb.Empty{}, nil
}

func (m *MetricsBackend) InstallerTemplate(context.Context, *emptypb.Empty) (*v1.InstallerTemplateResponse, error) {
	m.WaitForInit()

	return &v1.InstallerTemplateResponse{
		Template: `helm install opni-agent ` +
			`{{ arg "input" "Namespace" "+omitEmpty" "+default:opni-agent" "+format:-n {{ value }}" }} ` +
			`oci://docker.io/rancher/opni-agent --version=0.5.4 ` +
			`--set monitoring.enabled=true,token={{ .Token }},pin={{ .Pin }},address={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }} ` +
			`{{ arg "toggle" "Install Prometheus Operator" "+omitEmpty" "+default:false" "+format:--set kube-prometheus-stack.enabled={{ value }}" }} ` +
			`--create-namespace`,
	}, nil
}
