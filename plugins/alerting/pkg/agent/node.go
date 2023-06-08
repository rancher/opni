package agent

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
	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AlertingNode struct {
	capabilityv1.UnsafeNodeServer
	controlv1.UnsafeHealthServer

	ctx context.Context

	lg         *zap.SugaredLogger
	conditions healthpkg.ConditionTracker

	nodeMu               sync.RWMutex
	healthMu             sync.RWMutex
	nodeClient           node.NodeAlertingCapabilityClient
	identityClient       controlv1.IdentityClient
	healthListenerClient controlv1.HealthListenerClient

	configMu sync.RWMutex
	config   *node.AlertingCapabilityConfig

	listeners []drivers.ConfigPropagator
}

func NewAlertingNode(
	ctx context.Context,
	lg *zap.SugaredLogger,
	ct healthpkg.ConditionTracker,
) *AlertingNode {
	node := &AlertingNode{
		ctx:        ctx,
		lg:         lg,
		conditions: ct,
		listeners:  []drivers.ConfigPropagator{},
	}
	ct.AddListener(node.sendHealthUpdate)
	return node
}

var (
	_ capabilityv1.NodeServer = (*AlertingNode)(nil)
	_ controlv1.HealthServer  = (*AlertingNode)(nil)
)

func (a *AlertingNode) SyncNow(ctx context.Context, filter *capabilityv1.Filter) (*emptypb.Empty, error) {
	shouldAttempSync := slices.Contains(filter.GetCapabilityNames(), wellknown.CapabilityAlerting)
	if shouldAttempSync {
		a.nodeMu.RLock()
		defer a.nodeMu.RUnlock()

		if a.nodeClient == nil {
			return nil, status.Error(codes.Unavailable, "not connected to node server")
		}

		defer func() {
			ctx, ca := context.WithTimeout(a.ctx, 10*time.Second)
			go func() {
				defer ca()
				a.doSync(ctx)
			}()
		}()
		return &emptypb.Empty{}, nil
	}
	a.lg.Debug("capability sync ingnored due to filters")
	return &emptypb.Empty{}, nil
}

func (a *AlertingNode) addConfigListener(target drivers.ConfigPropagator) {
	a.listeners = append(a.listeners, target)
}

func (a *AlertingNode) doSync(ctx context.Context) {
	a.lg.Debug("syncing alerting node")
	a.nodeMu.RLock()
	defer a.nodeMu.RUnlock()

	if a.nodeClient == nil || a.healthListenerClient == nil {
		a.conditions.Set(healthpkg.CondConfigSync, health.StatusPending, "no client, skipping sync")
		return
	}

	a.configMu.RLock()
	syncResp, err := a.nodeClient.Sync(ctx, &node.SyncRequest{
		CurrentConfig: util.ProtoClone(a.config),
	})
	a.configMu.RUnlock()
	if err != nil {
		err := fmt.Errorf("error syncing metrics node : %w", err)
		a.conditions.Set(healthpkg.CondConfigSync, health.StatusFailure, err.Error())
		return
	}
	a.conditions.Clear(healthpkg.CondConfigSync)
	switch syncResp.ConfigStatus {
	case node.ConfigStatus_UpToDate:
		a.lg.Info("alerting node config is up to date")
	case node.ConfigStatus_NeedsUpdate:
		a.lg.Info("updating alerting node config")
		if err := a.updateConfig(ctx, syncResp.UpdatedConfig); err != nil {
			a.conditions.Set(healthpkg.CondNodeDriver, healthpkg.StatusFailure, err.Error())
			return
		} else {
			a.conditions.Clear(healthpkg.CondNodeDriver)
		}
	}
}

func (a *AlertingNode) updateConfig(ctx context.Context, config *node.AlertingCapabilityConfig) error {
	id, err := a.identityClient.Whoami(a.ctx, &emptypb.Empty{})
	if err != nil {
		a.lg.With("err", err).Errorf("failed to fetch node id %s", err)
		return err
	}

	a.nodeMu.Lock()
	defer a.nodeMu.Unlock()
	if !config.Enabled && len(config.Conditions) > 0 {
		a.conditions.Set(healthpkg.CondBackend, health.StatusDisabled, strings.Join(config.Conditions, ","))
	} else {
		a.conditions.Clear(healthpkg.CondBackend)
	}
	var eg util.MultiErrGroup
	for _, cfg := range a.listeners {
		cfg := cfg
		eg.Go(func() error {
			return cfg.ConfigureNode(id.Id, config)
		})
	}
	eg.Wait()

	a.config = config

	if err := eg.Error(); err != nil {
		a.config.Conditions = append(a.config.Conditions, err.Error())
		a.lg.With("err", err).Error("node configuration error")
		return err
	}
	return nil
}

func (a *AlertingNode) sendHealthUpdate() {
	a.healthMu.RLock()
	defer a.healthMu.RUnlock()

	if a.healthListenerClient == nil {
		return
	}

	health, err := a.GetHealth(a.ctx, &emptypb.Empty{})
	if err != nil {
		a.lg.With("err", err).Warn("failed to get node health")
		return
	}

	if _, err := a.healthListenerClient.UpdateHealth(a.ctx, health); err != nil {
		a.lg.With("err", err).Warn("failed to send health updates")
	} else {
		a.lg.Debug("send node health update")
	}
}

func (a *AlertingNode) setClients(
	nodeClient node.NodeAlertingCapabilityClient,
	identityClient controlv1.IdentityClient,
	healthListenerClient controlv1.HealthListenerClient,
) {
	a.nodeMu.Lock()
	a.nodeClient = nodeClient
	a.identityClient = identityClient
	a.nodeMu.Unlock()
	a.healthMu.Lock()
	a.healthListenerClient = healthListenerClient
	a.healthMu.Unlock()
	go func() {
		a.doSync(context.Background())
		a.sendHealthUpdate()
	}()
}

func (a *AlertingNode) GetHealth(ctx context.Context, _ *emptypb.Empty) (*corev1.Health, error) {
	conditions := a.conditions.List()
	sort.Strings(conditions)

	return &corev1.Health{
		Ready:      len(conditions) == 0,
		Conditions: conditions,
		Timestamp:  timestamppb.New(a.conditions.LastModified()),
	}, nil
}
