package gateway

/*
Orchestrator API implementation
*/

import (
	"context"

	"github.com/gogo/status"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/topology/pkg/apis/orchestrator"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) PutGraph(ctx context.Context, graph *orchestrator.TopologyGraph) (*emptypb.Empty, error) {
	// TODO : implement me
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetGraph(ctx context.Context, ref *corev1.Reference) (*orchestrator.TopologyGraph, error) {
	// TODO : implement me
	return nil, status.Error(codes.Unimplemented, "method not implemented")
}

func (p *Plugin) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*orchestrator.InstallStatus, error) {
	return &orchestrator.InstallStatus{
		State:   orchestrator.InstallState_Installed,
		Version: "0.1",
	}, nil
}
