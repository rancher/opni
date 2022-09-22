package gateway

import (
	"context"
	"math/rand"
	"sync"
	"time"

	agentv1 "github.com/rancher/opni/pkg/agent"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/util"
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

	// blocks until ctx is canceled
	// send a periodic sync request to the agent every 10-20 minutes
	f.runPeriodicSync(ctx, &capabilityv1.SyncRequest{
		Cluster: &corev1.Reference{
			Id: id,
		},
	}, 10*time.Minute, 10*time.Minute)

	f.mu.Lock()
	delete(f.activeAgents, id)
	f.logger.With("id", id).Debug("agent disconnected")
	f.mu.Unlock()
}

// Implements capabilityv1.NodeManagerServer
func (f *SyncRequester) RequestSync(ctx context.Context, req *capabilityv1.SyncRequest) (*emptypb.Empty, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	toSync := []agentv1.ClientSet{}

	if req.GetCluster().GetId() == "" {
		for _, clientSet := range f.activeAgents {
			toSync = append(toSync, clientSet)
		}
	} else {
		if clientSet, ok := f.activeAgents[req.GetCluster().GetId()]; ok {
			toSync = append(toSync, clientSet)
		}
	}

	for _, clientSet := range toSync {
		f.logger.With(
			"agentId", req.GetCluster().GetId(),
			"capabilities", req.GetFilter().GetCapabilityNames(),
		).Debug("sending sync request to agent")
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

func (f *SyncRequester) runPeriodicSync(ctx context.Context, req *capabilityv1.SyncRequest, period time.Duration, jitter time.Duration) {
	timer := time.NewTimer(period + time.Duration(rand.Int63n(int64(jitter))))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			go f.RequestSync(ctx, util.ProtoClone(req))
			timer.Reset(period + time.Duration(rand.Int63n(int64(jitter))))
		}
	}
}
