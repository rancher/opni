package agent

import (
	"context"
	"fmt"
	"sync"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MetricsNode struct {
	capabilityv1.UnsafeNodeServer
	controlv1.UnsafeHealthServer

	logger *zap.SugaredLogger

	clientMu sync.RWMutex
	client   node.NodeMetricsCapabilityClient

	configMu sync.RWMutex
	config   *node.MetricsCapabilityConfig

	listeners  []chan<- *node.MetricsCapabilityConfig
	conditions ConditionTracker
}

func NewMetricsNode(ct ConditionTracker, lg *zap.SugaredLogger) *MetricsNode {
	return &MetricsNode{
		logger:     lg,
		conditions: ct,
	}
}

func (m *MetricsNode) AddConfigListener(ch chan<- *node.MetricsCapabilityConfig) {
	m.listeners = append(m.listeners, ch)
}

func (m *MetricsNode) SetClient(client node.NodeMetricsCapabilityClient) {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()

	m.client = client

	go m.doSync(context.Background())
}

func (m *MetricsNode) Info(_ context.Context, _ *emptypb.Empty) (*capabilityv1.InfoResponse, error) {
	return &capabilityv1.InfoResponse{
		CapabilityName: wellknown.CapabilityMetrics,
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

	m.clientMu.RLock()
	defer m.clientMu.RUnlock()

	if m.client == nil {
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

	if !m.config.GetEnabled() {
		conditions = append(conditions, "Capability Disabled")
	}

	return &corev1.Health{
		Ready:      len(conditions) == 0,
		Conditions: conditions,
	}, nil
}

func (m *MetricsNode) doSync(ctx context.Context) {
	m.logger.Debug("syncing metrics node")
	m.clientMu.RLock()
	defer m.clientMu.RUnlock()

	if m.client == nil {
		m.conditions.Set(CondConfigSync, StatusPending, "no client, skipping sync")
		return
	}

	m.configMu.RLock()
	syncResp, err := m.client.Sync(ctx, &node.SyncRequest{
		CurrentConfig: util.ProtoClone(m.config),
	})
	m.configMu.RUnlock()

	if err != nil {
		err := fmt.Errorf("error syncing metrics node: %w", err)
		m.conditions.Set(CondConfigSync, StatusFailure, err.Error())
		return
	}

	switch syncResp.ConfigStatus {
	case node.ConfigStatus_UpToDate:
		m.logger.Info("metrics node config is up to date")
	case node.ConfigStatus_NeedsUpdate:
		m.logger.Info("updating metrics node config")
		m.updateConfig(syncResp.UpdatedConfig)
		go m.doSync(ctx)
	}

	m.conditions.Clear(CondConfigSync)
}

func (m *MetricsNode) updateConfig(config *node.MetricsCapabilityConfig) {
	m.configMu.Lock()
	defer m.configMu.Unlock()

	m.config = config

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
