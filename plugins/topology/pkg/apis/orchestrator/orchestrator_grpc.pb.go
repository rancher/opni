// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - ragu               v1.0.0
// source: github.com/rancher/opni/pkg/plugins/topology/pkg/apis/orchestrator/orchestrator.proto

package orchestrator

import (
	context "context"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// TopologyOrchestratorClient is the client API for TopologyOrchestrator service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type TopologyOrchestratorClient interface {
	Put(ctx context.Context, in *TopologyGraph, opts ...grpc.CallOption) (*emptypb.Empty, error)
	Get(ctx context.Context, in *v1.Reference, opts ...grpc.CallOption) (*TopologyGraph, error)
}

type topologyOrchestratorClient struct {
	cc grpc.ClientConnInterface
}

func NewTopologyOrchestratorClient(cc grpc.ClientConnInterface) TopologyOrchestratorClient {
	return &topologyOrchestratorClient{cc}
}

func (c *topologyOrchestratorClient) Put(ctx context.Context, in *TopologyGraph, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/orchestrator.TopologyOrchestrator/Put", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *topologyOrchestratorClient) Get(ctx context.Context, in *v1.Reference, opts ...grpc.CallOption) (*TopologyGraph, error) {
	out := new(TopologyGraph)
	err := c.cc.Invoke(ctx, "/orchestrator.TopologyOrchestrator/Get", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TopologyOrchestratorServer is the server API for TopologyOrchestrator service.
// All implementations must embed UnimplementedTopologyOrchestratorServer
// for forward compatibility
type TopologyOrchestratorServer interface {
	Put(context.Context, *TopologyGraph) (*emptypb.Empty, error)
	Get(context.Context, *v1.Reference) (*TopologyGraph, error)
	mustEmbedUnimplementedTopologyOrchestratorServer()
}

// UnimplementedTopologyOrchestratorServer must be embedded to have forward compatible implementations.
type UnimplementedTopologyOrchestratorServer struct {
}

func (UnimplementedTopologyOrchestratorServer) Put(context.Context, *TopologyGraph) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Put not implemented")
}
func (UnimplementedTopologyOrchestratorServer) Get(context.Context, *v1.Reference) (*TopologyGraph, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Get not implemented")
}
func (UnimplementedTopologyOrchestratorServer) mustEmbedUnimplementedTopologyOrchestratorServer() {}

// UnsafeTopologyOrchestratorServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to TopologyOrchestratorServer will
// result in compilation errors.
type UnsafeTopologyOrchestratorServer interface {
	mustEmbedUnimplementedTopologyOrchestratorServer()
}

func RegisterTopologyOrchestratorServer(s grpc.ServiceRegistrar, srv TopologyOrchestratorServer) {
	s.RegisterService(&TopologyOrchestrator_ServiceDesc, srv)
}

func _TopologyOrchestrator_Put_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TopologyGraph)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TopologyOrchestratorServer).Put(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/orchestrator.TopologyOrchestrator/Put",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TopologyOrchestratorServer).Put(ctx, req.(*TopologyGraph))
	}
	return interceptor(ctx, in, info, handler)
}

func _TopologyOrchestrator_Get_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(v1.Reference)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TopologyOrchestratorServer).Get(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/orchestrator.TopologyOrchestrator/Get",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TopologyOrchestratorServer).Get(ctx, req.(*v1.Reference))
	}
	return interceptor(ctx, in, info, handler)
}

// TopologyOrchestrator_ServiceDesc is the grpc.ServiceDesc for TopologyOrchestrator service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var TopologyOrchestrator_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "orchestrator.TopologyOrchestrator",
	HandlerType: (*TopologyOrchestratorServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Put",
			Handler:    _TopologyOrchestrator_Put_Handler,
		},
		{
			MethodName: "Get",
			Handler:    _TopologyOrchestrator_Get_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "github.com/rancher/opni/pkg/plugins/topology/pkg/apis/orchestrator/orchestrator.proto",
}
