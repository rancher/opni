package agent

import (
	"context"
	"sync"

	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"github.com/rancher/opni/plugins/topology/pkg/apis/node"
	"go.uber.org/zap"
)

type TopologyStreamer struct {
	logger     *zap.SugaredLogger
	conditions health.ConditionTracker

	remoteWriteClientMu sync.Mutex
	remoteWriteClient   remotewrite.RemoteWriteClient //TODO : implement our own
}

func NewTopologyStreamer(ct health.ConditionTracker, lg *zap.SugaredLogger) *TopologyStreamer {
	return &TopologyStreamer{
		logger:     lg,
		conditions: ct,
	}
}

func (s *TopologyStreamer) SetRemoteWriteClient(client remotewrite.RemoteWriteClient) {
	s.remoteWriteClientMu.Lock()
	defer s.remoteWriteClientMu.Unlock()
	s.remoteWriteClient = client
}

func (s *TopologyStreamer) Run(ctx context.Context, spec *node.TopologyCapabilitySpec) error {
	s.conditions.Set(CondTopologySync, health.StatusPending, "starting topology stream")
	defer s.conditions.Clear(CondTopologySync)
	lg := s.logger
	if spec == nil {
		lg.With("stream", "topology").Warn("no topology capability spec provided, setting defaults")

		// set some sensible defaults
	}

	//TODO : Implement me
	return nil
}
