package agent

import (
	"context"
	"fmt"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/plugins/import/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/import/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net/http"
	"sort"
	"sync"
)

var _ remoteread.RemoteReadAgentServer = (*ImportNode)(nil)

type ImportNode struct {
	remoteread.UnsafeRemoteReadAgentServer
	controlv1.UnsafeHealthServer
	capabilityv1.UnsafeNodeServer

	logger *zap.SugaredLogger

	conditions health.ConditionTracker

	targetRunnerMu sync.RWMutex
	targetRunner   TargetRunner

	nodeDriverMu sync.RWMutex
	nodeDriver   drivers.ImportNodeDriver

	nodeClientMu sync.RWMutex
	nodeClient   node.NodeMetricsCapabilityClient

	identityClientMu sync.RWMutex
	identityClient   controlv1.IdentityClient

	healthListenerClientMu sync.RWMutex
	healthListenerClient   controlv1.HealthListenerClient
}

func NewImportNode(ct health.ConditionTracker, lg *zap.SugaredLogger) *ImportNode {
	node := &ImportNode{
		logger:       lg,
		targetRunner: NewTargetRunner(lg),
		conditions:   ct,
	}

	node.conditions.AddListener(node.sendHealthUpdate)
	node.targetRunner.SetRemoteReaderClient(NewRemoteReader(&http.Client{}))

	return node
}

func (n *ImportNode) doSync(_ context.Context) {
	n.logger.Debug("syncing import node")
	n.nodeClientMu.RLock()
	defer n.nodeClientMu.RUnlock()

	if n.nodeClient == nil {
		n.conditions.Set(health.CondConfigSync, health.StatusPending, "no client, skippin gsync")
		return
	}

	n.conditions.Clear(health.CondConfigSync)
}

func (n *ImportNode) sendHealthUpdate() {
	// TODO this can be optimized to de-duplicate rapid updates
	n.healthListenerClientMu.RLock()
	defer n.healthListenerClientMu.RUnlock()

	if n.healthListenerClient != nil {
		health, err := n.GetHealth(context.TODO(), &emptypb.Empty{})
		if err != nil {
			n.logger.With(
				zap.Error(err),
			).Warn("failed to get node health")
			return
		}
		if _, err := n.healthListenerClient.UpdateHealth(context.TODO(), health); err != nil {
			n.logger.With(
				zap.Error(err),
			).Warn("failed to send node health update")
		} else {
			n.logger.Debug("sent node health update")
		}
	}
}

func (n *ImportNode) SetRemoteWriter(client clients.Locker[remotewrite.RemoteWriteClient]) {
	n.targetRunnerMu.Lock()
	defer n.targetRunnerMu.Unlock()

	n.targetRunner.SetRemoteWriteClient(client)
}

func (n *ImportNode) SetNodeDriver(d drivers.ImportNodeDriver) {
	n.nodeDriverMu.Lock()
	defer n.nodeDriverMu.Unlock()

	n.nodeDriver = d
}

func (n *ImportNode) SetNodeClient(client node.NodeMetricsCapabilityClient) {
	n.nodeClientMu.Lock()
	defer n.nodeClientMu.Unlock()

	n.nodeClient = client

	go n.doSync(context.Background())
}

func (n *ImportNode) SetIdentityClient(client controlv1.IdentityClient) {
	n.identityClientMu.Lock()
	defer n.identityClientMu.Unlock()

	n.identityClient = client
}

func (n *ImportNode) SetHealthListenerClient(client controlv1.HealthListenerClient) {
	n.healthListenerClientMu.Lock()
	n.healthListenerClient = client
	n.healthListenerClientMu.Unlock()
	n.sendHealthUpdate()
}

// capabilityv1.NodeServer

func (n *ImportNode) SyncNow(_ context.Context, filter *capabilityv1.Filter) (*emptypb.Empty, error) {
	if len(filter.CapabilityNames) > 0 {
		if !slices.Contains(filter.CapabilityNames, wellknown.CapabilityMetrics) {
			n.logger.Debug("ignoring sync request due to capability filter")
			return &emptypb.Empty{}, nil
		}
	}
	n.logger.Debug("received sync request")

	n.nodeClientMu.RLock()
	defer n.nodeClientMu.RUnlock()

	if n.nodeClient == nil {
		return nil, status.Errorf(codes.Unavailable, "not connected to node server")
	}

	defer func() {
		go n.doSync(context.Background())
	}()

	return &emptypb.Empty{}, nil
}

// ontrolv1.HealthServer

func (n *ImportNode) GetHealth(_ context.Context, _ *emptypb.Empty) (*corev1.Health, error) {
	conditions := n.conditions.List()

	sort.Strings(conditions)
	return &corev1.Health{
		Ready:      len(conditions) == 0,
		Conditions: conditions,
		Timestamp:  timestamppb.New(n.conditions.LastModified()),
	}, nil
}

// remoteread.RemoteReadAgentServer

func (n *ImportNode) Start(_ context.Context, request *remoteread.StartReadRequest) (*emptypb.Empty, error) {
	n.targetRunnerMu.Lock()
	defer n.targetRunnerMu.Unlock()

	if err := n.targetRunner.Start(request.Target, request.Query); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (n *ImportNode) Stop(_ context.Context, request *remoteread.StopReadRequest) (*emptypb.Empty, error) {
	n.targetRunnerMu.Lock()
	defer n.targetRunnerMu.Unlock()

	if err := n.targetRunner.Stop(request.Meta.Name); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (n *ImportNode) GetTargetStatus(_ context.Context, request *remoteread.TargetStatusRequest) (*remoteread.TargetStatus, error) {
	n.targetRunnerMu.RLock()
	defer n.targetRunnerMu.RUnlock()

	return n.targetRunner.GetStatus(request.Meta.Name)
}

func (n *ImportNode) Discover(ctx context.Context, request *remoteread.DiscoveryRequest) (*remoteread.DiscoveryResponse, error) {
	n.nodeDriverMu.RLock()
	defer n.nodeDriverMu.RUnlock()

	if n.nodeDriver == nil {
		n.logger.Warnf("no node driver available for discvoery")

		return &remoteread.DiscoveryResponse{
			Entries: []*remoteread.DiscoveryEntry{},
		}, nil
	}

	namespace := lo.FromPtrOr[string](request.Namespace, "")

	entries, err := n.nodeDriver.DiscoverPrometheuses(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("could not discover Prometheus instances: %w", err)
	}

	return &remoteread.DiscoveryResponse{
		Entries: entries,
	}, nil
}
