package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gogo/status"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/topology/pkg/apis/node"
	"golang.org/x/exp/slices"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
)

type TopologyNode struct {
	capabilityv1.UnsafeNodeServer
	controlv1.UnsafeHealthServer

	logger *zap.SugaredLogger

	clientMu sync.RWMutex
	client   node.NodeTopologyCapabilityClient

	configMu sync.RWMutex
	config   *node.TopologyCapabilityConfig

	listeners  []chan<- *node.TopologyCapabilityConfig
	conditions health.ConditionTracker
}

func NewTopologyNode(ct health.ConditionTracker, lg *zap.SugaredLogger) *TopologyNode {
	return &TopologyNode{
		logger:     lg,
		conditions: ct,
	}
}

func (t *TopologyNode) SetClient(client node.NodeTopologyCapabilityClient) {
	t.clientMu.Lock()
	defer t.clientMu.Unlock()

	t.client = client
	go t.doSync(context.Background())
}

func (t *TopologyNode) AddConfigListener(ch chan<- *node.TopologyCapabilityConfig) {
	t.listeners = append(t.listeners, ch)
}

func (t *TopologyNode) doSync(ctx context.Context) {
	t.logger.Debug("syncing metrics node")
	t.clientMu.RLock()
	defer t.clientMu.RUnlock()

	if t.client == nil {
		t.conditions.Set(health.CondConfigSync, health.StatusPending, "no client. skipping sync")
		return
	}

	t.configMu.RLock()
	syncResp, err := t.client.Sync(ctx, &node.SyncRequest{
		CurrentConfig: util.ProtoClone(t.config),
	})
	t.configMu.RUnlock()

	if err != nil {
		err := fmt.Errorf("error syncing metrics node: %w", err)
		t.conditions.Set(health.CondConfigSync, health.StatusFailure, err.Error())
		return
	}

	switch syncResp.ConfigStatus {
	case node.ConfigStatus_UpToDate:
		t.logger.Info("topology node is up to date")
	case node.ConfigStatus_NeedsUpdate:
		t.logger.Info("topology node needs update")
		t.updateConfig(syncResp.UpdatedConfig)
		go t.doSync(ctx)
	}

	t.conditions.Clear(health.CondConfigSync)
}

// Implements capabilityv1.NodeServer
func (t *TopologyNode) SyncNow(_ context.Context, req *capabilityv1.Filter) (*emptypb.Empty, error) {
	if len(req.GetCapabilityNames()) > 0 {
		if !slices.Contains(req.CapabilityNames, wellknown.CapabilityTopology) {
			t.logger.Debug("ignoring sync request due to capability filter")
			return &emptypb.Empty{}, nil
		}
	}
	t.logger.Debug("received sync request")
	t.clientMu.RLock()
	defer t.clientMu.RUnlock()

	if t.client == nil {
		return nil, status.Error(codes.Unavailable, "not connected to node server")
	}

	defer func() {
		go t.doSync(context.Background())
	}()
	return &emptypb.Empty{}, nil
}

// Implement controlv1.HealthServer
func (t *TopologyNode) GetHealth(_ context.Context, _ *emptypb.Empty) (*corev1.Health, error) {
	t.configMu.RLock()
	defer t.configMu.RUnlock()

	conditions := t.conditions.List()

	if t.config != nil {
		if !t.config.Enabled && len(t.config.Conditions) > 0 {
			conditions = append(conditions, fmt.Sprintf("Disabled: %s", strings.Join(t.config.Conditions, ", ")))
		}
	}

	return &corev1.Health{
		Ready:      len(conditions) == 0,
		Conditions: conditions,
	}, nil
}

func (t *TopologyNode) updateConfig(config *node.TopologyCapabilityConfig) {
	t.configMu.Lock()
	defer t.configMu.Unlock()

	t.config = config

	for _, ch := range t.listeners {
		clone := util.ProtoClone(config)
		select {
		case ch <- clone:
		default:
			t.logger.Warn("slow config update listener detected")
			ch <- clone
		}
	}
}
