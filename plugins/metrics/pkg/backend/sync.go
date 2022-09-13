package backend

import (
	"context"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
)

type NodeSyncManager struct {
	nodeStatusMu sync.RWMutex
	nodeStatus   map[string]*capabilityv1.NodeCapabilityStatus

	desiredNodeConfigMu sync.RWMutex
	desiredNodeConfig   map[string]*node.MetricsCapabilityConfig
}

func (m *NodeSyncManager) Sync(ctx context.Context, req *node.SyncRequest) (*node.SyncResponse, error) {
	id := cluster.StreamAuthorizedID(ctx)

	m.desiredNodeConfigMu.RLock()
	defer m.desiredNodeConfigMu.RUnlock()

	m.nodeStatusMu.RLock()
	defer m.nodeStatusMu.RUnlock()

	return buildResponse(req.CurrentConfig, m.desiredNodeConfig[id]), nil
}

func (m *NodeSyncManager) GetNodeStatus(id *corev1.Reference) (*capabilityv1.NodeCapabilityStatus, error) {
	m.nodeStatusMu.RLock()
	defer m.nodeStatusMu.RUnlock()

	if status, ok := m.nodeStatus[id.Id]; ok {
		return status, nil
	}

	return nil, status.Error(codes.NotFound, "no status has been reported for this node")
}

func (m *NodeSyncManager) SetDesiredConfiguration(id *corev1.Reference, config *node.MetricsCapabilityConfig) {
	m.desiredNodeConfigMu.Lock()
	defer m.desiredNodeConfigMu.Unlock()

	m.desiredNodeConfig[id.Id] = config
}

func (m *NodeSyncManager) ForgetNode(id string) {
	m.nodeStatusMu.Lock()
	defer m.nodeStatusMu.Unlock()

	delete(m.nodeStatus, id)
}

// the calling function must have exclusive ownership of both old and new
func buildResponse(old, new *node.MetricsCapabilityConfig) *node.SyncResponse {
	if proto.Equal(old, new) {
		return &node.SyncResponse{
			ConfigStatus: node.ConfigStatus_UpToDate,
		}
	}
	return &node.SyncResponse{
		ConfigStatus:  node.ConfigStatus_NeedsUpdate,
		UpdatedConfig: new,
	}
}
