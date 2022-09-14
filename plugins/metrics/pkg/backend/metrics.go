package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
)

type MetricsBackend struct {
	capabilityv1.UnsafeBackendServer
	node.UnsafeNodeMetricsCapabilityServer
	MetricsBackendConfig

	nodeStatusMu sync.RWMutex
	nodeStatus   map[string]*capabilityv1.NodeCapabilityStatus

	desiredNodeConfigMu sync.RWMutex
	desiredNodeConfig   map[string]*node.MetricsCapabilityConfig

	metricsutil.Initializer
}

var _ node.NodeMetricsCapabilityServer = (*MetricsBackend)(nil)

type MetricsBackendConfig struct {
	StorageBackend      storage.Backend               `validate:"required"`
	MgmtClient          managementv1.ManagementClient `validate:"required"`
	UninstallController *task.Controller              `validate:"required"`
}

func (m *MetricsBackend) Initialize(conf MetricsBackendConfig) {
	m.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		m.MetricsBackendConfig = conf
		m.nodeStatus = make(map[string]*capabilityv1.NodeCapabilityStatus)
		m.desiredNodeConfig = make(map[string]*node.MetricsCapabilityConfig)
	})
}

func (m *MetricsBackend) Info(ctx context.Context, _ *emptypb.Empty) (*capabilityv1.InfoResponse, error) {
	m.WaitForInit()

	return &capabilityv1.InfoResponse{
		CapabilityName: wellknown.CapabilityMetrics,
	}, nil
}

func (m *MetricsBackend) CanInstall(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	m.WaitForInit()

	return &emptypb.Empty{}, nil
}

func (m *MetricsBackend) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*emptypb.Empty, error) {
	m.WaitForInit()

	_, err := m.StorageBackend.UpdateCluster(ctx, req.Cluster,
		storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityMetrics)),
	)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (m *MetricsBackend) Status(ctx context.Context, req *capabilityv1.StatusRequest) (*capabilityv1.NodeCapabilityStatus, error) {
	m.WaitForInit()

	id, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
	if !ok {
		return nil, util.StatusError(codes.Unauthenticated)
	}

	m.nodeStatusMu.RLock()
	defer m.nodeStatusMu.RUnlock()

	if status, ok := m.nodeStatus[id]; ok {
		return status, nil
	}

	return nil, status.Error(codes.NotFound, "no status has been reported for this node")
}

func (m *MetricsBackend) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	m.WaitForInit()

	cluster, err := m.MgmtClient.GetCluster(ctx, req.Cluster)
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
		return nil, status.Error(codes.FailedPrecondition, "cluster does not have the reuqested capability")
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

func (m *MetricsBackend) UninstallStatus(ctx context.Context, cluster *corev1.Reference) (*corev1.TaskStatus, error) {
	m.WaitForInit()

	return m.UninstallController.TaskStatus(cluster.Id)
}

func (m *MetricsBackend) CancelUninstall(ctx context.Context, cluster *corev1.Reference) (*emptypb.Empty, error) {
	m.WaitForInit()

	m.UninstallController.CancelTask(cluster.Id)
	return &emptypb.Empty{}, nil
}

func (m *MetricsBackend) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	m.WaitForInit()

	return &capabilityv1.InstallerTemplateResponse{
		Template: `helm install opni-agent ` +
			`{{ arg "input" "Namespace" "+omitEmpty" "+default:opni-agent" "+format:-n {{ value }}" }} ` +
			`oci://docker.io/rancher/opni-helm --version=0.5.4 ` +
			`--set monitoring.enabled=true,token={{ .Token }},pin={{ .Pin }},address={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }} ` +
			`{{ arg "toggle" "Install Prometheus Operator" "+omitEmpty" "+default:false" "+format:--set kube-prometheus-stack.enabled={{ value }}" }} ` +
			`--create-namespace`,
	}, nil
}

type NodeSyncManager struct {
}

// Implements node.NodeMetricsCapabilityServer
func (m *MetricsBackend) Sync(ctx context.Context, req *node.SyncRequest) (*node.SyncResponse, error) {
	// todo: validate

	id, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
	if !ok {
		return nil, util.StatusError(codes.Unauthenticated)
	}

	m.desiredNodeConfigMu.RLock()
	defer m.desiredNodeConfigMu.RUnlock()

	m.nodeStatusMu.RLock()
	defer m.nodeStatusMu.RUnlock()

	status := m.nodeStatus[id]
	if status == nil {
		m.nodeStatus[id] = &capabilityv1.NodeCapabilityStatus{}
		status = m.nodeStatus[id]
	}

	status.Enabled = req.GetCurrentConfig().GetEnabled()
	status.LastSync = timestamppb.Now()

	return buildResponse(req.GetCurrentConfig(), m.desiredNodeConfig[id]), nil
}

func (m *MetricsBackend) SetDesiredConfiguration(id *corev1.Reference, config *node.MetricsCapabilityConfig) {
	m.desiredNodeConfigMu.Lock()
	defer m.desiredNodeConfigMu.Unlock()

	m.desiredNodeConfig[id.Id] = config
}

func (m *MetricsBackend) ForgetNode(id string) {
	m.nodeStatusMu.Lock()
	defer m.nodeStatusMu.Unlock()

	delete(m.nodeStatus, id)
}

// the calling function must have exclusive ownership of both old and new
func buildResponse(old, new *node.MetricsCapabilityConfig) *node.SyncResponse {
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
