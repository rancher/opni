package agent

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gogo/status"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	revision         string
	loadRevisionOnce sync.Once
)

func loadRevision() {
	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		panic("could not read build info")
	}
	for _, s := range buildInfo.Settings {
		if s.Key == "vcs.revision" {
			revision = s.Value
			return
		}
	}
	panic("could not find vcs.revision in build info")
}

type MetricsNode struct {
	capabilityv1.UnsafeNodeServer
	controlv1.UnsafeHealthServer

	clientMu sync.RWMutex
	client   node.NodeMetricsCapabilityClient
}

func (m *MetricsNode) SetClient(client node.NodeMetricsCapabilityClient) {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()

	m.client = client
}

func (p *MetricsNode) Info(ctx context.Context, _ *emptypb.Empty) (*capabilityv1.InfoResponse, error) {
	loadRevisionOnce.Do(loadRevision)
	return &capabilityv1.InfoResponse{
		CapabilityName: wellknown.CapabilityMetrics,
		Revision:       revision,
	}, nil
}

func (p *MetricsNode) Status(ctx context.Context, _ *emptypb.Empty) (*capabilityv1.NodeCapabilityStatus, error) {
	return &capabilityv1.NodeCapabilityStatus{}, nil
}

func (p *MetricsNode) SyncNow(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	p.clientMu.RLock()
	defer p.clientMu.RUnlock()

	if p.client == nil {
		return nil, status.Error(codes.Unavailable, "not connected to node server")
	}

	p.scheduleNextSync(time.Now())

	return &emptypb.Empty{}, nil
}

func (p *MetricsNode) GetHealth(ctx context.Context, _ *emptypb.Empty) (*corev1.Health, error) {
	return &corev1.Health{
		Ready:      false,
		Conditions: []string{"unimplemented"},
	}, nil
}

func (p *MetricsNode) scheduleNextSync(t time.Time) {

}
