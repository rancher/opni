package node

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type CapabilityConfig interface {
	proto.Message
	GetEnabled() bool
	GetConditions() []string
	SetConditions([]string)
}

type AbstractSyncResponse[T CapabilityConfig] interface {
	GetUpdatedConfig() T
	GetConfigStatus() corev1.ConfigStatus
}

type AbstractNodeSyncClient[T CapabilityConfig] interface {
	Sync(ctx context.Context, syncReq T, opt ...grpc.CallOption) (AbstractSyncResponse[T], error)
}

type ConfigPropagator[T CapabilityConfig] interface {
	ConfigureNode(nodeId string, config T) error
}

type HealthConfigSyncer[T CapabilityConfig] interface {
	AddConfigListener(p ConfigPropagator[T])
	Conditions() health.ConditionTracker
	SetClients(
		healthListenerClient controlv1.HealthListenerClient,
		nodeSyncClient AbstractNodeSyncClient[T],
		identityClient controlv1.IdentityClient,
	)
	capabilityv1.NodeServer
	controlv1.HealthServer
}

type defaultHealthConfigSyncer[
	T CapabilityConfig,
] struct {
	capabilityv1.UnsafeNodeServer
	controlv1.UnsafeHealthServer

	ctx        context.Context
	lg         *zap.SugaredLogger
	capability string

	configMu   sync.RWMutex
	config     T
	conditions health.ConditionTracker

	// required clients
	nodeMu               sync.RWMutex
	healthMu             sync.RWMutex
	idMu                 sync.RWMutex
	nodeSyncClient       AbstractNodeSyncClient[T]
	identityClient       controlv1.IdentityClient
	healthListenerClient controlv1.HealthListenerClient

	listenerMu sync.RWMutex
	listeners  []ConfigPropagator[T]
}

func NewDefaultHealthConfigSyncer[T CapabilityConfig](
	ctx context.Context,
	lg *zap.SugaredLogger,
	capability string,
	ct health.ConditionTracker,
) HealthConfigSyncer[T] {
	if !slices.Contains(wellknown.KnownCapabilities(), capability) {
		panic(fmt.Sprintf("unknown capability passed to health config syncer %s", capability))
	}
	node := &defaultHealthConfigSyncer[T]{
		ctx:        ctx,
		lg:         lg,
		conditions: ct,
		capability: capability,
		listeners:  []ConfigPropagator[T]{},
	}
	ct.AddListener(node.sendHealthUpdate)
	return node
}

func (s *defaultHealthConfigSyncer[T]) AddConfigListener(p ConfigPropagator[T]) {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	s.listeners = append(s.listeners, p)
}

func (s *defaultHealthConfigSyncer[T]) Conditions() health.ConditionTracker {
	return s.conditions
}

func (s *defaultHealthConfigSyncer[T]) SetClients(
	healthListenerClient controlv1.HealthListenerClient,
	nodeSyncClient AbstractNodeSyncClient[T],
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

func (s *defaultHealthConfigSyncer[T]) SyncNow(_ context.Context, req *capabilityv1.Filter) (*emptypb.Empty, error) {
	if len(req.CapabilityNames) > 0 {
		if !slices.Contains(req.CapabilityNames, s.capability) {
			s.lg.Debugf("ignoring sync request due to capability filter '%s'", s.capability)
			return &emptypb.Empty{}, nil
		}
		s.lg.Debug("received %s node sync request", s.capability)

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

func (s *defaultHealthConfigSyncer[T]) doSync(ctx context.Context) {
	s.lg.Debugf("syncing %s node", s.capability)
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
		s.lg.Infof("%s node config is up to date", s.capability)
	case corev1.ConfigStatus_NeedsUpdate:
		s.lg.Infof("%s updating node config", s.capability)
		if err := s.updateConfig(ctx, syncResp.GetUpdatedConfig()); err != nil {
			s.conditions.Set(health.CondNodeDriver, health.StatusFailure, err.Error())
			return
		} else {
			s.conditions.Clear(health.CondNodeDriver)
		}
	}
}

func (s *defaultHealthConfigSyncer[T]) updateConfig(ctx context.Context, config T) error {
	s.idMu.RLock()
	id, err := s.identityClient.Whoami(ctx, &emptypb.Empty{})
	if err != nil {
		s.lg.With(zap.Error(err)).Errorf("failed to fetch %s node id %s", s.capability, err)
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
	// TODO: this should ideally only be done if eg.Error() is nil, however
	// there is a risk of an infinite sync loop since we have to manually
	// re-sync when the driver status changes (see note in NewMetricsNode)
	// Once we replace the sync manager with delegates, we can safely return
	// errors from Sync and avoid the status condition workaround.
	s.config = config
	s.configMu.Unlock()

	if err := eg.Error(); err != nil {
		s.config.SetConditions(append(s.config.GetConditions(), err.Error()))
		s.lg.With(zap.Error(err)).Errorf("%s node configuration error", s.capability)
		return err
	}

	return nil
}

func (s *defaultHealthConfigSyncer[T]) hasRemoteHealthClient() bool {
	s.healthMu.RLock()
	s.healthMu.RUnlock()
	return s.healthListenerClient != nil
}

func (s *defaultHealthConfigSyncer[T]) hasIdentityClient() bool {
	s.idMu.RLock()
	defer s.idMu.RUnlock()
	return s.identityClient != nil
}

func (s *defaultHealthConfigSyncer[T]) hasNodeSyncClient() bool {
	s.nodeMu.RLock()
	defer s.nodeMu.RUnlock()
	return s.nodeSyncClient != nil
}

func (s *defaultHealthConfigSyncer[T]) GetHealth(_ context.Context, _ *emptypb.Empty) (*corev1.Health, error) {
	conditions := s.conditions.List()
	sort.Strings(conditions)

	return &corev1.Health{
		Ready:      len(conditions) == 0,
		Conditions: conditions,
		Timestamp:  timestamppb.New(s.conditions.LastModified()),
	}, nil
}

func (s *defaultHealthConfigSyncer[T]) sendHealthUpdate() {
	s.healthMu.RLock()
	defer s.healthMu.RUnlock()

	if !s.hasRemoteHealthClient() {
		s.lg.Warn("failed to send %s node health update, remote health client not set", s.capability)
		return
	}

	health, err := s.GetHealth(s.ctx, &emptypb.Empty{})
	if err != nil {
		s.lg.With("err", err).Warn("failed to get %s node health", s.capability)
		return
	}

	if _, err := s.healthListenerClient.UpdateHealth(s.ctx, health); err != nil {
		s.lg.With("err", err).Warn("failed to send %s node health updates", s.capability)
	} else {
		s.lg.Debug("send node health update")
	}
}
