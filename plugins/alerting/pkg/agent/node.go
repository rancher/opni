package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"slices"

	"log/slog"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ConfigPropagator interface {
	ConfigureNode(nodeId string, config *node.AlertingCapabilityConfig) error
}

type AlertingNode struct {
	capabilityv1.UnsafeNodeServer
	controlv1.UnsafeHealthServer

	ctx        context.Context
	lg         *slog.Logger
	capability string

	configMu   sync.RWMutex
	config     *node.AlertingCapabilityConfig
	conditions health.ConditionTracker

	// required clients
	nodeMu               sync.RWMutex
	healthMu             sync.RWMutex
	idMu                 sync.RWMutex
	nodeSyncClient       node.NodeAlertingCapabilityClient
	identityClient       controlv1.IdentityClient
	healthListenerClient controlv1.HealthListenerClient

	listenerMu sync.RWMutex
	listeners  []ConfigPropagator
}

func NewAlertingNode(
	ctx context.Context,
	lg *slog.Logger,
	ct health.ConditionTracker,
) *AlertingNode {
	node := &AlertingNode{
		ctx:        ctx,
		lg:         lg,
		conditions: ct,
		capability: wellknown.CapabilityAlerting,
		listeners:  []ConfigPropagator{},
	}
	ct.AddListener(node.sendHealthUpdate)
	return node
}

func (s *AlertingNode) AddConfigListener(p ConfigPropagator) {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	s.listeners = append(s.listeners, p)
}

func (s *AlertingNode) Conditions() health.ConditionTracker {
	return s.conditions
}

func (s *AlertingNode) SetClients(
	healthListenerClient controlv1.HealthListenerClient,
	nodeSyncClient node.NodeAlertingCapabilityClient,
	identityClient controlv1.IdentityClient,
) {
	s.healthMu.Lock()
	s.nodeMu.Lock()
	s.idMu.Lock()
	defer s.healthMu.Unlock()
	defer s.nodeMu.Unlock()
	defer s.idMu.Unlock()
	s.identityClient = identityClient
	s.nodeSyncClient = nodeSyncClient
	s.healthListenerClient = healthListenerClient

	go func() {
		s.doSync(s.ctx)
		s.sendHealthUpdate()
	}()
}

func (s *AlertingNode) SyncNow(_ context.Context, req *capabilityv1.Filter) (*emptypb.Empty, error) {
	if len(req.CapabilityNames) > 0 {
		if !slices.Contains(req.CapabilityNames, s.capability) {
			s.lg.Debug(fmt.Sprintf("ignoring sync request due to capability filter '%s'", s.capability))
			return &emptypb.Empty{}, nil
		}
		s.lg.Debug(fmt.Sprintf("received %s node sync request", s.capability))

		if !s.hasNodeSyncClient() {
			return nil, status.Error(codes.Unavailable, "not connected to node server")
		}

		defer func() {
			ctx, ca := context.WithTimeout(s.ctx, 10*time.Second)
			go func() {
				defer ca()
				s.doSync(ctx)
			}()
		}()
	}
	return &emptypb.Empty{}, nil
}

func (s *AlertingNode) doSync(ctx context.Context) {
	s.lg.Debug(fmt.Sprintf("syncing %s node", s.capability))
	if !s.hasNodeSyncClient() && !s.hasRemoteHealthClient() {
		s.conditions.Set(health.CondConfigSync, health.StatusPending, "no clients set, skipping")
		return
	}

	s.configMu.RLock()
	syncResp, err := s.nodeSyncClient.Sync(ctx, s.config)
	s.configMu.RUnlock()
	if err != nil {
		err := fmt.Errorf("error syncing  %s node : %w", s.capability, err)
		s.conditions.Set(health.CondConfigSync, health.StatusFailure, err.Error())
		return
	}

	s.conditions.Clear(health.CondConfigSync)
	switch syncResp.GetConfigStatus() {
	case corev1.ConfigStatus_UpToDate:
		s.lg.Info(fmt.Sprintf("%s node config is up to date", s.capability))
	case corev1.ConfigStatus_NeedsUpdate:
		s.lg.Info(fmt.Sprintf("%s updating node config", s.capability))
		if err := s.updateConfig(ctx, syncResp.GetUpdatedConfig()); err != nil {
			s.conditions.Set(health.CondNodeDriver, health.StatusFailure, err.Error())
			return
		} else {
			s.conditions.Clear(health.CondNodeDriver)
		}
	}
}

func (s *AlertingNode) updateConfig(ctx context.Context, config *node.AlertingCapabilityConfig) error {
	s.idMu.RLock()
	id, err := s.identityClient.Whoami(ctx, &emptypb.Empty{})
	if err != nil {
		s.lg.With(logger.Err(err)).Error(fmt.Sprintf("failed to fetch %s node id %s", s.capability, err))
		return err
	}
	s.idMu.RUnlock()

	if !config.GetEnabled() && len(config.GetConditions()) > 0 {
		s.conditions.Set(health.CondBackend, health.StatusDisabled, strings.Join(config.GetConditions(), ","))
	} else {
		s.conditions.Clear(health.CondBackend)
	}

	var eg util.MultiErrGroup
	for _, cfg := range s.listeners {
		cfg := cfg
		eg.Go(func() error {
			return cfg.ConfigureNode(id.Id, config)
		})
	}
	eg.Wait()
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if err := eg.Error(); err != nil {
		s.config.Conditions = (append(s.config.GetConditions(), err.Error()))
		s.lg.With(logger.Err(err)).Error(fmt.Sprintf("%s node configuration error", s.capability))
		return err
	} else {
		s.config = config
	}

	return nil
}

func (s *AlertingNode) hasRemoteHealthClient() bool {
	s.healthMu.RLock()
	defer s.healthMu.RUnlock()
	return s.healthListenerClient != nil
}

func (s *AlertingNode) hasIdentityClient() bool {
	s.idMu.RLock()
	defer s.idMu.RUnlock()
	return s.identityClient != nil
}

func (s *AlertingNode) hasNodeSyncClient() bool {
	s.nodeMu.RLock()
	defer s.nodeMu.RUnlock()
	return s.nodeSyncClient != nil
}

func (s *AlertingNode) GetHealth(_ context.Context, _ *emptypb.Empty) (*corev1.Health, error) {
	conditions := s.conditions.List()
	sort.Strings(conditions)

	return &corev1.Health{
		Ready:      len(conditions) == 0,
		Conditions: conditions,
		Timestamp:  timestamppb.New(s.conditions.LastModified()),
	}, nil
}

func (s *AlertingNode) sendHealthUpdate() {
	s.healthMu.RLock()
	defer s.healthMu.RUnlock()

	if !s.hasRemoteHealthClient() {
		s.lg.Warn(fmt.Sprintf("failed to send %s node health update, remote health client not set", s.capability))
		return
	}

	health, err := s.GetHealth(s.ctx, &emptypb.Empty{})
	if err != nil {
		s.lg.With(logger.Err(err)).Warn(fmt.Sprintf("failed to get %s node health", s.capability))
		return
	}

	if _, err := s.healthListenerClient.UpdateHealth(s.ctx, health); err != nil {
		s.lg.With(logger.Err(err)).Warn(fmt.Sprintf("failed to send %s node health updates", s.capability))
	} else {
		s.lg.Debug("send node health update")
	}
}
