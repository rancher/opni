package backend

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"

	"log/slog"

	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/go-cmp/cmp"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
)

type MetricsBackend struct {
	capabilityv1.UnsafeBackendServer
	node.UnsafeNodeMetricsCapabilityServer
	remoteread.UnsafeRemoteReadGatewayServer
	MetricsBackendConfig
	OpsBackend  *OpsServiceBackend
	NodeBackend *NodeServiceBackend

	nodeStatusMu sync.RWMutex
	nodeStatus   map[string]*capabilityv1.NodeCapabilityStatus

	// the stored remoteread.Target should never have their status populated
	remoteReadTargetMu sync.RWMutex
	remoteReadTargets  map[string]*remoteread.Target

	util.Initializer
}

var _ node.NodeMetricsCapabilityServer = (*MetricsBackend)(nil)
var _ remoteread.RemoteReadGatewayServer = (*MetricsBackend)(nil)

type MetricsAgentClientSet interface {
	agent.ClientSet
	remoteread.RemoteReadAgentClient
}

type MetricsBackendConfig struct {
	Logger              *slog.Logger                                    `validate:"required"`
	StorageBackend      storage.Backend                                 `validate:"required"`
	MgmtClient          managementv1.ManagementClient                   `validate:"required"`
	UninstallController *task.Controller                                `validate:"required"`
	ClusterDriver       drivers.ClusterDriver                           `validate:"required"`
	Delegate            streamext.StreamDelegate[MetricsAgentClientSet] `validate:"required"`
	KV                  *KVClients                                      `validate:"required"`
}

type KVClients struct {
	DefaultClusterConfigurationSpec storage.ValueStoreT[*cortexops.CapabilityBackendConfigSpec]
	DefaultCapabilitySpec           storage.ValueStoreT[*node.MetricsCapabilitySpec]
	NodeCapabilitySpecs             storage.KeyValueStoreT[*node.MetricsCapabilitySpec]
}

func (m *MetricsBackend) Initialize(conf MetricsBackendConfig) {
	m.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		m.MetricsBackendConfig = conf
		m.nodeStatus = make(map[string]*capabilityv1.NodeCapabilityStatus)
		m.remoteReadTargets = make(map[string]*remoteread.Target)
	})
}

func (m *MetricsBackend) requestNodeSync(ctx context.Context, target *corev1.Reference) error {
	if target == nil || target.Id == "" {
		panic("bug: target must be non-nil and have a non-empty ID. this logic was recently changed - please update the caller")
	}
	_, err := m.Delegate.
		WithTarget(target).
		SyncNow(ctx, &capabilityv1.Filter{CapabilityNames: []string{wellknown.CapabilityMetrics}})
	return err
}

func (m *MetricsBackend) broadcastNodeSync(ctx context.Context) {
	// keep any metadata in the context, but don't propagate cancellation
	ctx = context.WithoutCancel(ctx)
	var errs []error
	m.Delegate.
		WithBroadcastSelector(&corev1.ClusterSelector{}, func(reply any, msg *streamv1.BroadcastReplyList) error {
			for _, resp := range msg.GetResponses() {
				err := resp.GetReply().GetResponse().GetStatus().Err()
				if err != nil {
					target := resp.GetRef()
					errs = append(errs, status.Errorf(codes.Internal, "failed to sync agent %s: %v", target.GetId(), err))
				}
			}
			return nil
		}).
		SyncNow(ctx, &capabilityv1.Filter{
			CapabilityNames: []string{wellknown.CapabilityMetrics},
		})
	if len(errs) > 0 {
		m.Logger.With(
			logger.Err(errors.Join(errs...)),
		).Warn("one or more agents failed to sync; they may not be updated immediately")
	}
}

// Implements node.NodeMetricsCapabilityServer
func (m *MetricsBackend) Sync(ctx context.Context, req *node.SyncRequest) (*node.SyncResponse, error) {
	m.WaitForInit()
	id := cluster.StreamAuthorizedID(ctx)

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

	m.nodeStatusMu.Lock()
	defer m.nodeStatusMu.Unlock()

	status := m.nodeStatus[id]
	if status == nil {
		m.nodeStatus[id] = &capabilityv1.NodeCapabilityStatus{}
		status = m.nodeStatus[id]
	}

	status.Enabled = req.GetCurrentConfig().GetEnabled()
	status.Conditions = req.GetCurrentConfig().GetConditions()
	status.LastSync = timestamppb.Now()
	m.Logger.With(
		"id", id,
		"time", status.LastSync.AsTime(),
	).Debug("synced node")

	nodeSpec, err := m.getNodeSpecOrDefault(ctx, id)
	if err != nil {
		return nil, err
	}
	return buildResponse(req.GetCurrentConfig(), &node.MetricsCapabilityConfig{
		Enabled:    enabled,
		Conditions: conditions,
		Spec:       nodeSpec,
	}), nil
}

var (
	// The "default" default node spec. Exported for testing purposes.
	FallbackDefaultNodeSpec atomic.Pointer[node.MetricsCapabilitySpec]
)

func init() {
	FallbackDefaultNodeSpec.Store(&node.MetricsCapabilitySpec{
		Rules: &v1beta1.RulesSpec{
			Discovery: &v1beta1.DiscoverySpec{
				PrometheusRules: &v1beta1.PrometheusRulesSpec{},
			},
		},
		Driver: &node.MetricsCapabilitySpec_Prometheus{
			Prometheus: &node.PrometheusSpec{
				DeploymentStrategy: "externalPromOperator",
			},
		},
	})
}

func (m *MetricsBackend) getDefaultNodeSpec(ctx context.Context) (*node.MetricsCapabilitySpec, error) {
	nodeSpec, err := m.KV.DefaultCapabilitySpec.Get(ctx)
	if status.Code(err) == codes.NotFound {
		nodeSpec = FallbackDefaultNodeSpec.Load()
	} else if err != nil {
		m.Logger.With(logger.Err(err)).Error("failed to get default capability spec")
		return nil, status.Errorf(codes.Unavailable, "failed to get default capability spec: %v", err)
	}
	grpc.SetTrailer(ctx, node.DefaultConfigMetadata())
	return nodeSpec, nil
}

func (m *MetricsBackend) getNodeSpecOrDefault(ctx context.Context, id string) (*node.MetricsCapabilitySpec, error) {
	nodeSpec, err := m.KV.NodeCapabilitySpecs.Get(ctx, id)
	if status.Code(err) == codes.NotFound {
		return m.getDefaultNodeSpec(ctx)
	} else if err != nil {
		m.Logger.With(logger.Err(err)).Error("failed to get node capability spec")
		return nil, status.Errorf(codes.Unavailable, "failed to get node capability spec: %v", err)
	}
	// handle the case where an older config is now invalid: reset to factory default
	if err := nodeSpec.Validate(); err != nil {
		return m.getDefaultNodeSpec(ctx)
	}
	return nodeSpec, nil
}

// the calling function must have exclusive ownership of both old and new
func buildResponse(old, new *node.MetricsCapabilityConfig) *node.SyncResponse {
	oldIgnoreConditions := util.ProtoClone(old)
	if oldIgnoreConditions != nil {
		oldIgnoreConditions.Conditions = nil
	}
	newIgnoreConditions := util.ProtoClone(new)
	if newIgnoreConditions != nil {
		newIgnoreConditions.Conditions = nil
	}
	if cmp.Equal(oldIgnoreConditions, newIgnoreConditions, protocmp.Transform()) {
		return &node.SyncResponse{
			ConfigStatus: node.ConfigStatus_UpToDate,
		}
	}
	return &node.SyncResponse{
		ConfigStatus:  node.ConfigStatus_NeedsUpdate,
		UpdatedConfig: new,
	}
}
