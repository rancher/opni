package agent

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"github.com/samber/lo"

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

	listeners  []drivers.MetricsNodeConfigurator
	conditions health.ConditionTracker

	nodeDriverMu sync.RWMutex
	nodeDrivers  []drivers.MetricsNodeDriver
}

func NewMetricsNode(ct health.ConditionTracker, lg *zap.SugaredLogger) *MetricsNode {
	node := &MetricsNode{
		logger:       lg,
		conditions:   ct,
		targetRunner: NewTargetRunner(lg),
	}
	node.conditions.AddListener(node.sendHealthUpdate)
	node.targetRunner.SetRemoteReaderClient(NewRemoteReader(&http.Client{}))

	// FIXME: this is a hack, update the old sync code to use delegates instead
	node.conditions.AddListener(func(key string) {
		if key == CondNodeDriver {
			node.logger.Info("forcing sync due to node driver status change")
			go func() {
				node.doSync(context.TODO())
			}()
		}
	})

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

func (m *MetricsNode) AddConfigListener(ch drivers.MetricsNodeConfigurator) {
	m.listeners = append(m.listeners, ch)
}

func (m *MetricsNode) SetClients(
	nodeClient node.NodeMetricsCapabilityClient,
	identityClient controlv1.IdentityClient,
	healthListenerClient controlv1.HealthListenerClient,
) {
	m.nodeClientMu.Lock()
	m.nodeClient = nodeClient
	m.nodeClientMu.Unlock()

	m.identityClientMu.Lock()
	m.identityClient = identityClient
	m.identityClientMu.Unlock()

	m.healthListenerClientMu.Lock()
	m.healthListenerClient = healthListenerClient
	m.healthListenerClientMu.Unlock()

	go func() {
		m.doSync(context.Background())
		m.sendHealthUpdate()
	}()
}

func (m *MetricsNode) SetRemoteWriter(client clients.Locker[remotewrite.RemoteWriteClient]) {
	m.targetRunnerMu.Lock()
	defer m.targetRunnerMu.Unlock()

	m.targetRunner.SetRemoteWriteClient(client)
}

func (m *MetricsNode) AddNodeDriver(driver drivers.MetricsNodeDriver) {
	m.nodeDriverMu.Lock()
	defer m.nodeDriverMu.Unlock()

	m.nodeDrivers = append(m.nodeDrivers, driver)
}

func (m *MetricsNode) Info(_ context.Context, _ *emptypb.Empty) (*capabilityv1.Details, error) {
	return &capabilityv1.Details{
		Name:    wellknown.CapabilityMetrics,
		Source:  "plugin_metrics",
		Drivers: drivers.NodeDrivers.List(),
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
		ctx, ca := context.WithTimeout(context.Background(), 10*time.Second)
		go func() {
			defer ca()
			m.doSync(ctx)
		}()
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

func (m *MetricsNode) Start(_ context.Context, request *remoteread.StartReadRequest) (*emptypb.Empty, error) {
	m.targetRunnerMu.Lock()
	defer m.targetRunnerMu.Unlock()

	if err := m.targetRunner.Start(request.Target, request.Query); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (m *MetricsNode) Stop(_ context.Context, request *remoteread.StopReadRequest) (*emptypb.Empty, error) {
	m.targetRunnerMu.Lock()
	defer m.targetRunnerMu.Unlock()

	if err := m.targetRunner.Stop(request.Meta.Name); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (m *MetricsNode) GetTargetStatus(_ context.Context, request *remoteread.TargetStatusRequest) (*remoteread.TargetStatus, error) {
	m.targetRunnerMu.RLock()
	defer m.targetRunnerMu.RUnlock()

	return m.targetRunner.GetStatus(request.Meta.Name)
}

func (m *MetricsNode) Discover(ctx context.Context, request *remoteread.DiscoveryRequest) (*remoteread.DiscoveryResponse, error) {
	m.nodeDriverMu.RLock()
	defer m.nodeDriverMu.RUnlock()

	if len(m.nodeDrivers) == 0 {
		m.logger.Warnf("no node driver available for discvoery")

		return &remoteread.DiscoveryResponse{
			Entries: []*remoteread.DiscoveryEntry{},
		}, nil
	}

	namespace := lo.FromPtrOr(request.Namespace, "")

	var allEntries []*remoteread.DiscoveryEntry
	for _, driver := range m.nodeDrivers {
		entries, err := driver.DiscoverPrometheuses(ctx, namespace)
		if err != nil {
			return nil, fmt.Errorf("could not discover Prometheus instances: %w", err)
		}
		allEntries = append(allEntries, entries...)
	}

	return &remoteread.DiscoveryResponse{
		Entries: allEntries,
	}, nil
}

func (m *MetricsNode) doSync(ctx context.Context) {
	m.logger.Debug("syncing metrics node")
	m.nodeClientMu.RLock()
	defer m.nodeClientMu.RUnlock()
	m.identityClientMu.RLock()
	defer m.identityClientMu.RUnlock()

	if m.nodeClient == nil {
		m.conditions.Set(health.CondConfigSync, health.StatusPending, "no client, skipping sync")
		return
	}

	if m.identityClient == nil {
		m.conditions.Set(health.CondConfigSync, health.StatusPending, "no client, skipping sync")
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
	m.conditions.Clear(health.CondConfigSync)

	switch syncResp.ConfigStatus {
	case node.ConfigStatus_UpToDate:
		m.logger.Info("metrics node config is up to date")
	case node.ConfigStatus_NeedsUpdate:
		m.logger.Info("updating metrics node config")
		if err := m.updateConfig(ctx, syncResp.UpdatedConfig); err != nil {
			m.conditions.Set(health.CondNodeDriver, health.StatusFailure, err.Error())
			return
		} else {
			m.conditions.Clear(health.CondNodeDriver)
		}
	}
}

// requires identityClientMu to be held (either R or W)
func (m *MetricsNode) updateConfig(ctx context.Context, config *node.MetricsCapabilityConfig) error {
	id, err := m.identityClient.Whoami(context.Background(), &emptypb.Empty{})
	if err != nil {
		m.logger.With(zap.Error(err)).Errorf("error fetching node id", err)
		return err
	}

	if !m.configMu.TryLock() {
		m.logger.Debug("waiting on a previous config update to finish...")
		m.configMu.Lock()
	}
	defer m.configMu.Unlock()
	if !config.Enabled && len(config.Conditions) > 0 {
		m.conditions.Set(health.CondBackend, health.StatusDisabled, strings.Join(config.Conditions, ", "))
	} else {
		m.conditions.Clear(health.CondBackend)
	}

	var eg util.MultiErrGroup
	for _, cfg := range m.listeners {
		cfg := cfg
		eg.Go(func() error {
			return cfg.ConfigureNode(id.Id, config)
		})
	}

	eg.Wait()

	// TODO: this should ideally only be done if eg.Error() is nil, however
	// there is a risk of an infinite sync loop since we have to manually
	// re-sync when the driver status changes (see note in NewMetricsNode)
	// Once we replace the sync manager with delegates, we can safely return
	// errors from Sync and avoid the status condition workaround.
	m.config = config

	if err := eg.Error(); err != nil {
		m.config.Conditions = append(config.Conditions, err.Error())
		m.logger.With(zap.Error(err)).Error("node configuration error")
		return err
	}
	return nil
}
