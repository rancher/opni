package agent

import (
	"context"
	"runtime/debug"
	"sync"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
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

func (p *Plugin) Info(ctx context.Context, _ *emptypb.Empty) (*capabilityv1.InfoResponse, error) {
	loadRevisionOnce.Do(loadRevision)
	return &capabilityv1.InfoResponse{
		CapabilityName: wellknown.CapabilityMetrics,
		Revision:       revision,
	}, nil
}

func (p *Plugin) Status(ctx context.Context, _ *emptypb.Empty) (*capabilityv1.NodeCapabilityStatus, error) {
	return &capabilityv1.NodeCapabilityStatus{}, nil
}

func (p *Plugin) SyncNow(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetHealth(ctx context.Context, _ *emptypb.Empty) (*corev1.Health, error) {
	return &corev1.Health{
		Ready:      false,
		Conditions: []string{"unimplemented"},
	}, nil
}
