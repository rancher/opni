package backend

import (
	"context"

	"github.com/google/go-cmp/cmp"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
)

type NodeServiceBackend struct {
	node.UnsafeNodeConfigurationServer
	*MetricsBackend
}

func (m *NodeServiceBackend) syncAllNodes() error {
	_, err := m.Delegate.WithBroadcastSelector(&corev1.ClusterSelector{}, stream.DiscardReplies).
		SyncNow(context.Background(), &capabilityv1.Filter{
			CapabilityNames: []string{wellknown.CapabilityMetrics},
		}, grpc.WaitForReady(false))
	return err
}

func (m *NodeServiceBackend) GetDefaultNodeConfiguration(ctx context.Context, _ *emptypb.Empty) (*node.MetricsCapabilitySpec, error) {
	m.WaitForInit()
	return m.getDefaultNodeSpec(ctx)
}

func (m *NodeServiceBackend) GetNodeConfiguration(ctx context.Context, node *corev1.Reference) (*node.MetricsCapabilitySpec, error) {
	m.WaitForInit()
	return m.getNodeSpecOrDefault(ctx, node.GetId())
}

func (m *NodeServiceBackend) SetDefaultNodeConfiguration(ctx context.Context, conf *node.MetricsCapabilitySpec) (*emptypb.Empty, error) {
	m.WaitForInit()
	var empty node.MetricsCapabilitySpec
	if cmp.Equal(conf, &empty, protocmp.Transform()) {
		if err := m.KV.DefaultCapabilitySpec.Delete(ctx); err != nil {
			return nil, err
		}
		m.broadcastNodeSync(ctx)
		return &emptypb.Empty{}, nil
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	if err := m.KV.DefaultCapabilitySpec.Put(ctx, conf); err != nil {
		return nil, err
	}
	m.broadcastNodeSync(ctx)
	return &emptypb.Empty{}, nil
}

func (m *NodeServiceBackend) SetNodeConfiguration(ctx context.Context, req *node.NodeConfigRequest) (*emptypb.Empty, error) {
	m.WaitForInit()
	if req.Spec == nil {
		if err := m.KV.NodeCapabilitySpecs.Delete(ctx, req.Node.GetId()); err != nil {
			return nil, err
		}

		return &emptypb.Empty{}, nil
	}
	if err := req.Spec.Validate(); err != nil {
		return nil, err
	}

	if err := m.KV.NodeCapabilitySpecs.Put(ctx, req.Node.GetId(), req.GetSpec()); err != nil {
		return nil, err
	}

	m.requestNodeSync(ctx, req.Node)
	return &emptypb.Empty{}, nil
}
