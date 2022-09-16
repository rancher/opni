package gateway

import (
	"context"
	"sync"

	agentv1 "github.com/rancher/opni/pkg/agent"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type SyncRequester struct {
	capabilityv1.UnsafeNodeManagerServer
	mu           sync.RWMutex
	activeAgents map[string]agentv1.ClientSet
	logger       *zap.SugaredLogger
}

func NewSyncRequester(lg *zap.SugaredLogger) *SyncRequester {
	return &SyncRequester{
		activeAgents: make(map[string]agentv1.ClientSet),
		logger:       lg.Named("sync"),
	}
}

func (f *SyncRequester) HandleAgentConnection(ctx context.Context, clientSet agentv1.ClientSet) {
	f.mu.Lock()
	id := cluster.StreamAuthorizedID(ctx)
	f.activeAgents[id] = clientSet
	f.logger.With("id", id).Debug("agent connected")
	f.mu.Unlock()

	<-ctx.Done()

	f.mu.Lock()
	delete(f.activeAgents, id)
	f.logger.With("id", id).Debug("agent disconnected")
	f.mu.Unlock()
}

// Implements capabilityv1.NodeManagerServer
func (f *SyncRequester) RequestSync(ctx context.Context, req *capabilityv1.SyncRequest) (*emptypb.Empty, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if clientSet, ok := f.activeAgents[req.GetCluster().GetId()]; ok {
		f.logger.With(
			"agentId", req.GetCluster().GetId(),
			"capabilities", req.GetFilter().GetCapabilityNames(),
		).Info("sending sync request to agent")
		_, err := clientSet.SyncNow(ctx, req.GetFilter())
		if err != nil {
			f.logger.With(
				zap.Error(err),
			).Warn("sync request failed")
			return &emptypb.Empty{}, err
		}
		return &emptypb.Empty{}, nil
	}
	return &emptypb.Empty{}, status.Error(codes.NotFound, "agent is not connected")
}
