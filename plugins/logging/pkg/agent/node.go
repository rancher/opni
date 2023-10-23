package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"slices"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/logging/apis/node"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log/slog"
)

type LoggingNode struct {
	capabilityv1.UnsafeNodeServer
	controlv1.UnsafeHealthServer

	logger *slog.Logger

	clientMu sync.RWMutex
	client   node.NodeLoggingCapabilityClient

	configMu sync.RWMutex
	config   *node.LoggingCapabilityConfig

	listeners  []chan<- *node.LoggingCapabilityConfig
	conditions health.ConditionTracker
}

func NewLoggingNode(ct health.ConditionTracker, lg *slog.Logger) *LoggingNode {
	return &LoggingNode{
		logger:     lg,
		conditions: ct,
	}
}

func (l *LoggingNode) AddConfigListener(ch chan<- *node.LoggingCapabilityConfig) {
	l.listeners = append(l.listeners, ch)
}

func (l *LoggingNode) SetClient(client node.NodeLoggingCapabilityClient) {
	l.clientMu.Lock()
	defer l.clientMu.Unlock()

	l.client = client

	go l.doSync(context.Background())
}

func (l *LoggingNode) Info(_ context.Context, _ *emptypb.Empty) (*capabilityv1.Details, error) {
	return &capabilityv1.Details{
		Name:    wellknown.CapabilityLogs,
		Source:  "plugin_logging",
		Drivers: drivers.NodeDrivers.List(),
	}, nil
}

// Implements capabilityv1.NodeServer
func (l *LoggingNode) SyncNow(_ context.Context, req *capabilityv1.Filter) (*emptypb.Empty, error) {
	if len(req.CapabilityNames) > 0 {
		if !slices.Contains(req.CapabilityNames, wellknown.CapabilityLogs) {
			l.logger.Debug("ignoring sync request due to capability filter")
			return &emptypb.Empty{}, nil
		}
	}
	l.logger.Debug("received sync request")

	l.clientMu.RLock()
	defer l.clientMu.RUnlock()

	if l.client == nil {
		return nil, status.Error(codes.Unavailable, "not connected to node server")
	}

	defer func() {
		go l.doSync(context.Background())
	}()

	return &emptypb.Empty{}, nil
}

// Implements controlv1.HealthServer
func (l *LoggingNode) GetHealth(_ context.Context, _ *emptypb.Empty) (*corev1.Health, error) {
	l.configMu.RLock()
	defer l.configMu.RUnlock()

	conditions := l.conditions.List()

	if l.config != nil {
		if !l.config.Enabled && len(l.config.Conditions) > 0 {
			conditions = append(conditions, fmt.Sprintf("Disabled: %s", strings.Join(l.config.Conditions, ", ")))
		}
	}

	sort.Strings(conditions)
	return &corev1.Health{
		Timestamp:  timestamppb.Now(),
		Ready:      len(conditions) == 0,
		Conditions: conditions,
	}, nil
}

func (l *LoggingNode) doSync(ctx context.Context) {
	l.logger.Debug("syncing logging node")
	l.clientMu.RLock()
	defer l.clientMu.RUnlock()

	if l.client == nil {
		l.conditions.Set(health.CondConfigSync, health.StatusPending, "no client, skipping sync")
		return
	}

	l.configMu.RLock()
	syncResp, err := l.client.Sync(ctx, &node.SyncRequest{
		CurrentConfig: util.ProtoClone(l.config),
	})
	l.configMu.RUnlock()

	if err != nil {
		err := fmt.Errorf("error syncing logging node: %w", err)
		l.conditions.Set(health.CondConfigSync, health.StatusFailure, err.Error())
		return
	}

	switch syncResp.ConfigStatus {
	case node.ConfigStatus_UpToDate:
		l.logger.Info("logging node config is up to date")
	case node.ConfigStatus_NeedsUpdate:
		l.logger.Info("updating logging node config")
		l.updateConfig(syncResp.GetUpdatedConfig())
	}

	l.conditions.Clear(health.CondConfigSync)
}

func (l *LoggingNode) updateConfig(config *node.LoggingCapabilityConfig) {
	l.configMu.Lock()
	defer l.configMu.Unlock()

	l.config = config

	if !l.config.Enabled && len(l.config.Conditions) > 0 {
		l.conditions.Set(health.CondBackend, health.StatusDisabled, strings.Join(l.config.Conditions, ", "))
	} else {
		l.conditions.Clear(health.CondBackend)
	}

	for _, ch := range l.listeners {
		clone := util.ProtoClone(config)
		select {
		case ch <- clone:
		default:
			l.logger.Warn("slow config update listener detected")
			ch <- clone
		}
	}
}
