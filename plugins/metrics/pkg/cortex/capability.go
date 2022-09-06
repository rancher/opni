package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (p *Plugin) Info(context.Context, *emptypb.Empty) (*capabilityv1.InfoResponse, error) {
	return &capabilityv1.InfoResponse{
		CapabilityName: wellknown.CapabilityMetrics,
	}, nil
}

func (p *Plugin) CanInstall(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (p *Plugin) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*emptypb.Empty, error) {
	_, err := p.storageBackend.Get().UpdateCluster(ctx, req.Cluster,
		storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityMetrics)),
	)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) Status(ctx context.Context, req *capabilityv1.StatusRequest) (*capabilityv1.NodeCapabilityStatus, error) {
	mgr := p.syncMgr.Load()
	if mgr == nil {
		return nil, util.StatusError(codes.Unavailable)
	}
	return mgr.GetNodeStatus(req.Cluster)
}

func (p *Plugin) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	cluster, err := p.mgmtApi.Get().GetCluster(ctx, req.Cluster)
	if err != nil {
		return nil, err
	}

	defaultOpts := capabilityv1.DefaultUninstallOptions{}
	if strings.TrimSpace(req.Options) != "" {
		if err := json.Unmarshal([]byte(req.Options), &defaultOpts); err != nil {
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
			stat, err := p.uninstallController.Get().TaskStatus(cluster.Id)
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
	_, err = p.storageBackend.Get().UpdateCluster(ctx, cluster.Reference(), func(c *corev1.Cluster) {
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

	md := uninstall.TimestampedMetadata{
		DefaultUninstallOptions: defaultOpts,
		DeletionTimestamp:       now.AsTime(),
	}
	err = p.uninstallController.Get().LaunchTask(req.Cluster.Id, task.WithMetadata(md))
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) UninstallStatus(ctx context.Context, cluster *corev1.Reference) (*corev1.TaskStatus, error) {
	return p.uninstallController.Get().TaskStatus(cluster.Id)
}

func (p *Plugin) CancelUninstall(ctx context.Context, cluster *corev1.Reference) (*emptypb.Empty, error) {
	p.uninstallController.Get().CancelTask(cluster.Id)
	return &emptypb.Empty{}, nil
}

func (p *Plugin) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return &capabilityv1.InstallerTemplateResponse{
		Template: `helm install opni-agent ` +
			`{{ arg "input" "Namespace" "+omitEmpty" "+default:opni-agent" "+format:-n {{ value }}" }} ` +
			`oci://docker.io/rancher/opni-helm --version=0.5.4 ` +
			`--set monitoring.enabled=true,token={{ .Token }},pin={{ .Pin }},address={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }} ` +
			`{{ arg "toggle" "Install Prometheus Operator" "+omitEmpty" "+default:false" "+format:--set kube-prometheus-stack.enabled={{ value }}" }} ` +
			`--create-namespace`,
	}, nil
}
