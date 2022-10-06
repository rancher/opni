package gateway

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/topology/pkg/apis/orchestrator"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) Put(ctx context.Context, graph *orchestrator.TopologyGraph) (*emptypb.Empty, error) {
	// TODO : implement me
	return nil, nil
}

func (p *Plugin) Get(ctx context.Context, ref *corev1.Reference) (*orchestrator.TopologyGraph, error) {
	// TODO : implement me
	return nil, nil
}
