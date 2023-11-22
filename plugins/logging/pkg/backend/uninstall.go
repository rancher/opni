package backend

import (
	"context"
	"fmt"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (b *LoggingBackend) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	b.WaitForInit()

	cluster, err := b.MgmtClient.GetCluster(ctx, req.Agent)
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
		if cap.Name != wellknown.CapabilityLogs {
			continue
		}
		exists = true

		// check for a previous stale task that may not have been cleaned up
		if cap.DeletionTimestamp != nil {
			// if the deletion timestamp is set and the task is not completed, error
			stat, err := b.UninstallController.TaskStatus(cluster.Id)
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
		return nil, status.Error(codes.FailedPrecondition, "cluster does not have the reuqested capability")
	}

	now := timestamppb.Now()
	_, err = b.StorageBackend.UpdateCluster(ctx, cluster.Reference(), func(c *opnicorev1.Cluster) {
		for _, cap := range c.Metadata.Capabilities {
			if cap.Name == wellknown.CapabilityLogs {
				cap.DeletionTimestamp = now
				break
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update cluster metadata: %v", err)
	}

	b.requestNodeSync(ctx, req.Agent)

	md := uninstall.TimestampedMetadata{
		DefaultUninstallOptions: defaultOpts,
		DeletionTimestamp:       now.AsTime(),
	}
	err = b.UninstallController.LaunchTask(req.Agent.Id, task.WithMetadata(md))
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (b *LoggingBackend) UninstallStatus(_ context.Context, req *capabilityv1.UninstallStatusRequest) (*opnicorev1.TaskStatus, error) {
	b.WaitForInit()
	return b.UninstallController.TaskStatus(req.Agent.GetId())
}

func (b *LoggingBackend) CancelUninstall(_ context.Context, req *capabilityv1.CancelUninstallRequest) (*emptypb.Empty, error) {
	b.WaitForInit()
	b.UninstallController.CancelTask(req.Agent.GetId())
	return &emptypb.Empty{}, nil
}
