package backend

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/go-cmp/cmp"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
)

type MetricsBackend struct {
	capabilityv1.UnsafeBackendServer
	node.UnsafeNodeMetricsCapabilityServer
	cortexops.UnsafeCortexOpsServer
	MetricsBackendConfig

	nodeStatusMu sync.RWMutex
	nodeStatus   map[string]*capabilityv1.NodeCapabilityStatus

	desiredNodeSpecMu sync.RWMutex
	desiredNodeSpec   map[string]*node.MetricsCapabilitySpec

	util.Initializer
}

var _ node.NodeMetricsCapabilityServer = (*MetricsBackend)(nil)
var _ cortexops.CortexOpsServer = (*MetricsBackend)(nil)

type MetricsBackendConfig struct {
	Logger              *zap.SugaredLogger             `validate:"required"`
	StorageBackend      storage.Backend                `validate:"required"`
	MgmtClient          managementv1.ManagementClient  `validate:"required"`
	NodeManagerClient   capabilityv1.NodeManagerClient `validate:"required"`
	UninstallController *task.Controller               `validate:"required"`
	ClusterDriver       drivers.ClusterDriver          `validate:"required"`
}

func (m *MetricsBackend) Initialize(conf MetricsBackendConfig) {
	m.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		m.MetricsBackendConfig = conf
		m.nodeStatus = make(map[string]*capabilityv1.NodeCapabilityStatus)
		m.desiredNodeSpec = make(map[string]*node.MetricsCapabilitySpec)
	})
}

func (m *MetricsBackend) Info(_ context.Context, _ *emptypb.Empty) (*capabilityv1.Details, error) {
	// Info must not block
	var drivers []string
	if m.Initialized() {
		drivers = append(drivers, m.ClusterDriver.Name())
	}

	return &capabilityv1.Details{
		Name:    wellknown.CapabilityMetrics,
		Source:  "plugin_metrics",
		Drivers: drivers,
	}, nil
}

func (m *MetricsBackend) CanInstall(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	m.WaitForInit()

	return &emptypb.Empty{}, nil
}

func (m *MetricsBackend) canInstall(ctx context.Context) error {
	stat, err := m.ClusterDriver.GetClusterStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return status.Error(codes.Unavailable, err.Error())
	}
	switch stat.State {
	case cortexops.InstallState_Updating, cortexops.InstallState_Installed:
		// ok
	case cortexops.InstallState_NotInstalled, cortexops.InstallState_Uninstalling:
		return status.Error(codes.Unavailable, fmt.Sprintf("cortex cluster is not installed"))
	case cortexops.InstallState_Unknown:
		fallthrough
	default:
		return status.Error(codes.Internal, fmt.Sprintf("unknown cortex cluster state"))
	}
	return nil
}

func (m *MetricsBackend) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	m.WaitForInit()

	var warningErr error
	if err := m.canInstall(ctx); err != nil {
		if !req.IgnoreWarnings {
			return &capabilityv1.InstallResponse{
				Status:  capabilityv1.InstallResponseStatus_Error,
				Message: err.Error(),
			}, nil
		} else {
			warningErr = err
		}
	}

	_, err := m.StorageBackend.UpdateCluster(ctx, req.Cluster,
		storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityMetrics)),
	)
	if err != nil {
		return nil, err
	}

	m.requestNodeSync(ctx, req.Cluster)

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

func (m *MetricsBackend) Status(ctx context.Context, req *capabilityv1.StatusRequest) (*capabilityv1.NodeCapabilityStatus, error) {
	m.WaitForInit()

	m.nodeStatusMu.RLock()
	defer m.nodeStatusMu.RUnlock()

	if status, ok := m.nodeStatus[req.Cluster.Id]; ok {
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

	var defaultOpts capabilityv1.DefaultUninstallOptions
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
	m.requestNodeSync(ctx, req.Cluster)

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

	m.requestNodeSync(ctx, cluster)
	return &emptypb.Empty{}, nil
}

func (m *MetricsBackend) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	m.WaitForInit()

	return &capabilityv1.InstallerTemplateResponse{
		Template: `helm install opni-agent ` +
			`{{ arg "input" "Namespace" "+omitEmpty" "+default:opni-agent" "+format:-n {{ value }}" }} ` +
			`oci://docker.io/rancher/opni-agent --version=0.5.4 ` +
			`--set monitoring.enabled=true,token={{ .Token }},pin={{ .Pin }},address={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }} ` +
			`{{ arg "toggle" "Install Prometheus Operator" "+omitEmpty" "+default:false" "+format:--set kube-prometheus-stack.enabled={{ value }}" }} ` +
			`--create-namespace`,
	}, nil
}

func (m *MetricsBackend) requestNodeSync(ctx context.Context, cluster *corev1.Reference) {
	_, err := m.NodeManagerClient.RequestSync(ctx, &capabilityv1.SyncRequest{
		Cluster: cluster,
		Filter: &capabilityv1.Filter{
			CapabilityNames: []string{wellknown.CapabilityMetrics},
		},
	})

	name := cluster.GetId()
	if name == "" {
		name = "(all)"
	}
	if err != nil {
		m.Logger.With(
			"cluster", name,
			"capability", wellknown.CapabilityMetrics,
			zap.Error(err),
		).Warn("failed to request node sync; nodes may not be updated immediately")
		return
	}
	m.Logger.With(
		"cluster", name,
		"capability", wellknown.CapabilityMetrics,
	).Info("node sync requested")
}

// Implements node.NodeMetricsCapabilityServer
func (m *MetricsBackend) Sync(ctx context.Context, req *node.SyncRequest) (*node.SyncResponse, error) {
	m.WaitForInit()
	// todo: validate

	id, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
	if !ok {
		return nil, util.StatusError(codes.Unauthenticated)
	}

	// look up the cluster and check if the capability is installed
	cluster, err := m.StorageBackend.GetCluster(ctx, &corev1.Reference{
		Id: id,
	})
	if err != nil {
		return nil, err
	}
	var enabled bool
	for _, cap := range cluster.GetCapabilities() {
		if cap.Name == wellknown.CapabilityMetrics {
			enabled = (cap.DeletionTimestamp == nil)
		}
	}
	var conditions []string
	if enabled {
		// auto-disable if cortex is not installed
		if err := m.ClusterDriver.ShouldDisableNode(cluster.Reference()); err != nil {
			reason := status.Convert(err).Message()
			m.Logger.With(
				"reason", reason,
			).Info("disabling metrics capability for node")
			enabled = false
			conditions = append(conditions, reason)
		}
	}

	m.desiredNodeSpecMu.RLock()
	defer m.desiredNodeSpecMu.RUnlock()

	m.nodeStatusMu.Lock()
	defer m.nodeStatusMu.Unlock()

	status := m.nodeStatus[id]
	if status == nil {
		m.nodeStatus[id] = &capabilityv1.NodeCapabilityStatus{}
		status = m.nodeStatus[id]
	}

	status.Enabled = req.GetCurrentConfig().GetEnabled()
	status.LastSync = timestamppb.Now()

	// todo: allow for this to be configurable
	return buildResponse(req.GetCurrentConfig(), &node.MetricsCapabilityConfig{
		Enabled:    enabled,
		Conditions: conditions,
		Spec: &node.MetricsCapabilitySpec{
			Rules: &v1beta1.RulesSpec{
				Discovery: &v1beta1.DiscoverySpec{
					PrometheusRules: &v1beta1.PrometheusRulesSpec{},
				},
			},
			Prometheus: &node.PrometheusSpec{
				DeploymentStrategy: "externalPromOperator",
			},
		},
	}), nil
	// return buildResponse(req.GetCurrentConfig(), &node.MetricsCapabilityConfig{
	// 	Enabled: enabled,
	// 	Spec:    m.desiredNodeSpec[id],
	// }), nil
}

// the calling function must have exclusive ownership of both old and new
func buildResponse(old, new *node.MetricsCapabilityConfig) *node.SyncResponse {
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

// Cortex Ops Backend

func (m *MetricsBackend) GetClusterConfiguration(ctx context.Context, in *emptypb.Empty) (*cortexops.ClusterConfiguration, error) {
	m.WaitForInit()

	return m.ClusterDriver.GetClusterConfiguration(ctx, in)
}

func (m *MetricsBackend) ConfigureCluster(ctx context.Context, in *cortexops.ClusterConfiguration) (*emptypb.Empty, error) {
	m.WaitForInit()

	defer m.requestNodeSync(ctx, &corev1.Reference{})
	return m.ClusterDriver.ConfigureCluster(ctx, in)
}

func (m *MetricsBackend) GetClusterStatus(ctx context.Context, in *emptypb.Empty) (*cortexops.InstallStatus, error) {
	m.WaitForInit()

	return m.ClusterDriver.GetClusterStatus(ctx, in)
}

func (m *MetricsBackend) UninstallCluster(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
	m.WaitForInit()

	clusters, err := m.StorageBackend.ListClusters(ctx, &corev1.LabelSelector{}, corev1.MatchOptions_Default)
	if err != nil {
		return nil, err
	}
	clustersWithCapability := []string{}
	for _, c := range clusters.GetItems() {
		if capabilities.Has(c, capabilities.Cluster(wellknown.CapabilityMetrics)) {
			clustersWithCapability = append(clustersWithCapability, c.Id)
		}
	}
	if len(clustersWithCapability) > 0 {
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("metrics capability is still installed on the following clusters: %s", strings.Join(clustersWithCapability, ", ")))
	}
	defer m.requestNodeSync(ctx, &corev1.Reference{})
	return m.ClusterDriver.UninstallCluster(ctx, in)
}
