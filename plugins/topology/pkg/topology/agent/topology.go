package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"time"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/topology/graph"
	"github.com/rancher/opni/plugins/topology/pkg/apis/node"
	"github.com/rancher/opni/plugins/topology/pkg/apis/stream"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BatchingConfig struct {
	maxSize int
	timeout time.Duration
}

type TopologyStreamer struct {
	logger     *zap.SugaredLogger
	conditions health.ConditionTracker

	v                chan client.Object
	eventWatchClient client.WithWatch

	identityClientMu       sync.Mutex
	identityClient         controlv1.IdentityClient
	topologyStreamClientMu sync.Mutex
	topologyStreamClient   stream.RemoteTopologyClient
}

func NewTopologyStreamer(ct health.ConditionTracker, lg *zap.SugaredLogger) *TopologyStreamer {
	return &TopologyStreamer{
		// FIXME: reintroduce this when we want to monitor kubernetes events
		// eventWatchClient: util.Must(client.NewWithWatch(
		// 	util.Must(rest.InClusterConfig()),
		// 	client.Options{
		// 		Scheme: apis.NewScheme(),
		// 	})),
		logger:     lg,
		conditions: ct,
	}
}

func (s *TopologyStreamer) SetTopologyStreamClient(client stream.RemoteTopologyClient) {
	s.topologyStreamClientMu.Lock()
	defer s.topologyStreamClientMu.Unlock()
	s.topologyStreamClient = client
}

func (s *TopologyStreamer) SetIdentityClient(identityClient controlv1.IdentityClient) {
	s.identityClientMu.Lock()
	defer s.identityClientMu.Unlock()
	s.identityClient = identityClient

}

func (s *TopologyStreamer) Run(ctx context.Context, spec *node.TopologyCapabilitySpec) error {
	lg := s.logger
	if spec == nil {
		lg.With("stream", "topology").Warn("no topology capability spec provided, setting defaults")

		// set some sensible defaults
	}
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	// blocking action
	for {
		select {
		case <-ctx.Done():
			lg.With(
				zap.Error(ctx.Err()),
			).Warn("topology stream closing")
			return nil
		case <-tick.C:
			// this will panic when not  in a cluster : ruh roh
			//  need to refactor to cluster driver

			g, err := graph.TraverseTopology(graph.NewRuntimeFactory())
			if err != nil {
				lg.With(
					zap.Error(err),
				).Error("Could not construct topology graph")
			}
			var b bytes.Buffer
			err = json.NewEncoder(&b).Encode(g)
			if err != nil {
				lg.With(
					zap.Error(err),
				).Warn("failed to encode kubernetes graph")
				continue
			}
			s.identityClientMu.Lock()
			thisCluster, err := s.identityClient.Whoami(ctx, &emptypb.Empty{})
			if err != nil {
				lg.With(
					zap.Error(err),
				).Warn("failed to get cluster identity")
				continue
			}
			s.identityClientMu.Unlock()

			s.topologyStreamClientMu.Lock()
			_, err = s.topologyStreamClient.Push(ctx, &stream.Payload{
				Graph: &stream.TopologyGraph{
					ClusterId: thisCluster,
					Data:      b.Bytes(),
					Repr:      stream.GraphRepr_KubectlGraph,
				},
			})
			s.topologyStreamClientMu.Unlock()
		}
	}
}
