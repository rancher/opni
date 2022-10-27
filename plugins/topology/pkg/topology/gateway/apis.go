package gateway

/*
Orchestrator API implementation
*/

import (
	"context"
	"encoding/json"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/topology/store"
	"github.com/rancher/opni/plugins/topology/pkg/apis/orchestrator"
	"github.com/rancher/opni/plugins/topology/pkg/apis/representation"
	"github.com/steveteuber/kubectl-graph/pkg/graph"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) GetGraph(ctx context.Context, ref *corev1.Reference) (*representation.TopologyGraph, error) {
	// TODO(topology) : implement me
	return nil, status.Error(codes.Unimplemented, "method not implemented")
}

func (p *Plugin) RenderGraph(ctx context.Context, clusterRef *corev1.Reference) (*representation.GraphSVG, error) {
	if !p.topologyRemoteWrite.Initialized() {
		return nil, status.Error(codes.Unavailable, "topology remote write not initialized")
	}
	ctxCa, cancel := context.WithCancel(ctx)
	defer cancel()
	mgr, err := p.nc.GetContext(ctxCa)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	obj, err := store.GetTopologyObjectStore(mgr)
	if err != nil {
		return nil, err
	}
	objInfo, err := obj.GetInfo(store.NewClusterKey(clusterRef))
	if err != nil {
		return nil, err
	}
	reprKey := objInfo.Headers.Get(store.ReprHeaderKey)
	if reprKey == "" {
		return nil, status.Error(
			codes.Internal,
			"no representation header found for the cluster",
		)
	}
	graphObj, err := obj.Get(store.NewClusterKey(clusterRef))
	if err != nil {
		return nil, err
	}
	var g graph.Graph
	if err := json.NewDecoder(graphObj).Decode(&g); err != nil {
		return nil, err
	}
	switch reprKey {
	case representation.GraphRepr_KubectlGraph.String():
		return nil, status.Error(codes.Unimplemented, "rendering kubectl graph not implemented yet")
	default:
		return nil, status.Error(codes.Internal, "invalid representation key")
	}
}

func (p *Plugin) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*orchestrator.InstallStatus, error) {
	return &orchestrator.InstallStatus{
		State:   orchestrator.InstallState_Installed,
		Version: "0.1",
	}, nil
}
