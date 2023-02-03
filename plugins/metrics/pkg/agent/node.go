package agent

import (
	"context"
	"fmt"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"sort"
	"strings"
	"sync"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MetricsNode struct {
	capabilityv1.UnsafeNodeServer
	controlv1.UnsafeHealthServer

	// we only need a subset of the methods
	remoteread.UnsafeRemoteReadAgentServer

	logger *zap.SugaredLogger

	nodeClientMu sync.RWMutex
	nodeClient   node.NodeMetricsCapabilityClient

	identityClientMu sync.RWMutex
	identityClient   controlv1.IdentityClient

	healthListenerClientMu sync.RWMutex
	healthListenerClient   controlv1.HealthListenerClient

	targetRunnerMu sync.RWMutex
	targetRunner   TargetRunner

	configMu sync.RWMutex
	config   *node.MetricsCapabilityConfig

	listeners  []chan<- *node.MetricsCapabilityConfig
	conditions health.ConditionTracker
}

func NewMetricsNode(ct health.ConditionTracker, lg *zap.SugaredLogger) *MetricsNode {
	node := &MetricsNode{
		logger:       lg,
		conditions:   ct,
		targetRunner: NewTargetRunner(lg),
	}
	node.conditions.AddListener(node.sendHealthUpdate)
	return node
}

func (m *MetricsNode) sendHealthUpdate() {
	// TODO this can be optimized to de-duplicate rapid updates
	m.healthListenerClientMu.RLock()
	defer m.healthListenerClientMu.RUnlock()
	if m.healthListenerClient != nil {
		health, err := m.GetHealth(context.TODO(), &emptypb.Empty{})
		if err != nil {
			m.logger.With(
				zap.Error(err),
			).Warn("failed to get node health")
			return
		}
		if _, err := m.healthListenerClient.UpdateHealth(context.TODO(), health); err != nil {
			m.logger.With(
				zap.Error(err),
			).Warn("failed to send node health update")
		} else {
			m.logger.Debug("sent node health update")
		}
	}
}

func (m *MetricsNode) AddConfigListener(ch chan<- *node.MetricsCapabilityConfig) {
	m.listeners = append(m.listeners, ch)
}

func (m *MetricsNode) SetNodeClient(client node.NodeMetricsCapabilityClient) {
	m.nodeClientMu.Lock()
	defer m.nodeClientMu.Unlock()

	m.nodeClient = client

	go m.doSync(context.Background())
}

func (m *MetricsNode) SetIdentityClient(client controlv1.IdentityClient) {
	m.identityClientMu.Lock()
	defer m.identityClientMu.Unlock()

	m.identityClient = client
}

func (m *MetricsNode) SetHealthListenerClient(client controlv1.HealthListenerClient) {
	m.healthListenerClientMu.Lock()
	m.healthListenerClient = client
	m.healthListenerClientMu.Unlock()
	m.sendHealthUpdate()
}

func (m *MetricsNode) SetRemoteWriter(client clients.Locker[remotewrite.RemoteWriteClient]) {
	m.targetRunnerMu.Lock()
	defer m.targetRunnerMu.Unlock()

	m.targetRunner.SetRemoteWriteClient(client)
}

func (m *MetricsNode) Info(_ context.Context, _ *emptypb.Empty) (*capabilityv1.Details, error) {
	return &capabilityv1.Details{
		Name:    wellknown.CapabilityMetrics,
		Source:  "plugin_metrics",
		Drivers: drivers.ListNodeDrivers(),
	}, nil
}

// Implements capabilityv1.NodeServer

func (m *MetricsNode) SyncNow(_ context.Context, req *capabilityv1.Filter) (*emptypb.Empty, error) {
	if len(req.CapabilityNames) > 0 {
		if !slices.Contains(req.CapabilityNames, wellknown.CapabilityMetrics) {
			m.logger.Debug("ignoring sync request due to capability filter")
			return &emptypb.Empty{}, nil
		}
	}
	m.logger.Debug("received sync request")

	m.nodeClientMu.RLock()
	defer m.nodeClientMu.RUnlock()

	if m.nodeClient == nil {
		return nil, status.Error(codes.Unavailable, "not connected to node server")
	}

	defer func() {
		go m.doSync(context.Background())
	}()

	return &emptypb.Empty{}, nil
}

// Implements controlv1.HealthServer

func (m *MetricsNode) GetHealth(_ context.Context, _ *emptypb.Empty) (*corev1.Health, error) {
	m.configMu.RLock()
	defer m.configMu.RUnlock()

	conditions := m.conditions.List()

	sort.Strings(conditions)
	return &corev1.Health{
		Ready:      len(conditions) == 0,
		Conditions: conditions,
		Timestamp:  timestamppb.New(m.conditions.LastModified()),
	}, nil
}

// Start Implements remoteread.RemoteReadServer

func (m *MetricsNode) Start(ctx context.Context, request *remoteread.StartReadRequest) (*emptypb.Empty, error) {
	m.targetRunnerMu.Lock()
	defer m.targetRunnerMu.Unlock()

	if err := m.targetRunner.Start(request.Target, request.Query); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (m *MetricsNode) Stop(ctx context.Context, request *remoteread.StopReadRequest) (*emptypb.Empty, error) {
	m.targetRunnerMu.Lock()
	defer m.targetRunnerMu.Unlock()

	if err := m.targetRunner.Stop(request.Meta.Name); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (m *MetricsNode) GetTargetStatus(ctx context.Context, request *remoteread.TargetStatusRequest) (*remoteread.TargetStatus, error) {
	m.targetRunnerMu.RLock()
	defer m.targetRunnerMu.RUnlock()

	return m.targetRunner.GetStatus(request.Meta.Name)
}

func (m *MetricsNode) Discover(ctx context.Context, request *remoteread.DiscoveryRequest) (*remoteread.DiscoveryResponse, error) {
	discoverer, err := NewPrometheusDiscoverer(DiscovererConfig{
		RESTConfig: nil,
		Context:    ctx,
		Logger:     m.logger.Named("prom-discovery"),
	})

	if err != nil {
		m.logger.With(
			"capability", wellknown.CapabilityMetrics,
			zap.Error(err),
		).Errorf("could not create prometheus discoverer: %s", err)

		return nil, fmt.Errorf("could not create prometheus discoverer: %s", err)
	}

	entries, err := discoverer.Discover()
	if err != nil {
		return nil, fmt.Errorf("could not discover Prometheues instances: %w", err)
	}

	return &remoteread.DiscoveryResponse{
		Entries: entries,
	}, nil
}

func (m *MetricsNode) doSync(ctx context.Context) {
	m.logger.Debug("syncing metrics node")
	m.nodeClientMu.RLock()
	defer m.nodeClientMu.RUnlock()

	if m.nodeClient == nil {
		m.conditions.Set(health.CondConfigSync, health.StatusPending, "no promClient, skipping sync")
		return
	}

	m.configMu.RLock()
	syncResp, err := m.nodeClient.Sync(ctx, &node.SyncRequest{
		CurrentConfig: util.ProtoClone(m.config),
	})
	m.configMu.RUnlock()

	if err != nil {
		err := fmt.Errorf("error syncing metrics node: %w", err)
		m.conditions.Set(health.CondConfigSync, health.StatusFailure, err.Error())
		return
	}

	switch syncResp.ConfigStatus {
	case node.ConfigStatus_UpToDate:
		m.logger.Info("metrics node config is up to date")
	case node.ConfigStatus_NeedsUpdate:
		m.logger.Info("updating metrics node config")
		m.updateConfig(syncResp.UpdatedConfig)
	}

	m.conditions.Clear(health.CondConfigSync)
}

func (m *MetricsNode) updateConfig(config *node.MetricsCapabilityConfig) {
	m.configMu.Lock()
	defer m.configMu.Unlock()

	m.config = config

	if !m.config.Enabled && len(m.config.Conditions) > 0 {
		m.conditions.Set(health.CondBackend, health.StatusDisabled, strings.Join(m.config.Conditions, ", "))
	} else {
		m.conditions.Clear(health.CondBackend)
	}

	for _, ch := range m.listeners {
		clone := util.ProtoClone(config)
		select {
		case ch <- clone:
		default:
			m.logger.Warn("slow config update listener detected")
			ch <- clone
		}
	}
}
