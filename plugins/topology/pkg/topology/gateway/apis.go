package gateway

/*
Orchestrator API implementation
*/

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/topology/pkg/apis/orchestrator"
	"github.com/rancher/opni/plugins/topology/pkg/apis/representation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) StoreGraph(ctx context.Context, graph *representation.TopologyGraph) (*emptypb.Empty, error) {
	// TODO(topology) : implement me
	return nil, status.Error(codes.Unimplemented, "method not implemented")
}

func (p *Plugin) GetGraph(ctx context.Context, ref *corev1.Reference) (*representation.TopologyGraph, error) {
	// TODO(topology) : implement me
	return nil, status.Error(codes.Unimplemented, "method not implemented")
}

func (p *Plugin) RenderGraph(ctx context.Context, graph *representation.TopologyGraph) (*representation.GraphHtml, error) {
	// TODO(topology) : implement me
	return nil, status.Error(codes.Unimplemented, "method not implemented")
}

func (p *Plugin) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*orchestrator.InstallStatus, error) {
	return &orchestrator.InstallStatus{
		State:   orchestrator.InstallState_Installed,
		Version: "0.1",
	}, nil
}
