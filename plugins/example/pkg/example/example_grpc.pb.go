// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - ragu               v1.0.0
// source: github.com/rancher/opni/plugins/example/pkg/example/example.proto

package example

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

const (
	ExampleAPIExtension_Echo_FullMethodName = "/example.ExampleAPIExtension/Echo"
)

// ExampleAPIExtensionClient is the client API for ExampleAPIExtension service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ExampleAPIExtensionClient interface {
	Echo(ctx context.Context, in *EchoRequest, opts ...grpc.CallOption) (*EchoResponse, error)
}

type exampleAPIExtensionClient struct {
	cc grpc.ClientConnInterface
}

func NewExampleAPIExtensionClient(cc grpc.ClientConnInterface) ExampleAPIExtensionClient {
	return &exampleAPIExtensionClient{cc}
}

func (c *exampleAPIExtensionClient) Echo(ctx context.Context, in *EchoRequest, opts ...grpc.CallOption) (*EchoResponse, error) {
	out := new(EchoResponse)
	err := c.cc.Invoke(ctx, ExampleAPIExtension_Echo_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ExampleAPIExtensionServer is the server API for ExampleAPIExtension service.
// All implementations must embed UnimplementedExampleAPIExtensionServer
// for forward compatibility
type ExampleAPIExtensionServer interface {
	Echo(context.Context, *EchoRequest) (*EchoResponse, error)
	mustEmbedUnimplementedExampleAPIExtensionServer()
}

// UnimplementedExampleAPIExtensionServer must be embedded to have forward compatible implementations.
type UnimplementedExampleAPIExtensionServer struct {
}

func (UnimplementedExampleAPIExtensionServer) Echo(context.Context, *EchoRequest) (*EchoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Echo not implemented")
}
func (UnimplementedExampleAPIExtensionServer) mustEmbedUnimplementedExampleAPIExtensionServer() {}

// UnsafeExampleAPIExtensionServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ExampleAPIExtensionServer will
// result in compilation errors.
type UnsafeExampleAPIExtensionServer interface {
	mustEmbedUnimplementedExampleAPIExtensionServer()
}

func RegisterExampleAPIExtensionServer(s grpc.ServiceRegistrar, srv ExampleAPIExtensionServer) {
	s.RegisterService(&ExampleAPIExtension_ServiceDesc, srv)
}

func _ExampleAPIExtension_Echo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(EchoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExampleAPIExtensionServer).Echo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ExampleAPIExtension_Echo_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExampleAPIExtensionServer).Echo(ctx, req.(*EchoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// ExampleAPIExtension_ServiceDesc is the grpc.ServiceDesc for ExampleAPIExtension service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var ExampleAPIExtension_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "example.ExampleAPIExtension",
	HandlerType: (*ExampleAPIExtensionServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Echo",
			Handler:    _ExampleAPIExtension_Echo_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "github.com/rancher/opni/plugins/example/pkg/example/example.proto",
}

const (
	ExampleUnaryExtension_Hello_FullMethodName = "/example.ExampleUnaryExtension/Hello"
)

// ExampleUnaryExtensionClient is the client API for ExampleUnaryExtension service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ExampleUnaryExtensionClient interface {
	Hello(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*EchoResponse, error)
}

type exampleUnaryExtensionClient struct {
	cc grpc.ClientConnInterface
}

func NewExampleUnaryExtensionClient(cc grpc.ClientConnInterface) ExampleUnaryExtensionClient {
	return &exampleUnaryExtensionClient{cc}
}

func (c *exampleUnaryExtensionClient) Hello(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*EchoResponse, error) {
	out := new(EchoResponse)
	err := c.cc.Invoke(ctx, ExampleUnaryExtension_Hello_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ExampleUnaryExtensionServer is the server API for ExampleUnaryExtension service.
// All implementations must embed UnimplementedExampleUnaryExtensionServer
// for forward compatibility
type ExampleUnaryExtensionServer interface {
	Hello(context.Context, *emptypb.Empty) (*EchoResponse, error)
	mustEmbedUnimplementedExampleUnaryExtensionServer()
}

// UnimplementedExampleUnaryExtensionServer must be embedded to have forward compatible implementations.
type UnimplementedExampleUnaryExtensionServer struct {
}

func (UnimplementedExampleUnaryExtensionServer) Hello(context.Context, *emptypb.Empty) (*EchoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Hello not implemented")
}
func (UnimplementedExampleUnaryExtensionServer) mustEmbedUnimplementedExampleUnaryExtensionServer() {}

// UnsafeExampleUnaryExtensionServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ExampleUnaryExtensionServer will
// result in compilation errors.
type UnsafeExampleUnaryExtensionServer interface {
	mustEmbedUnimplementedExampleUnaryExtensionServer()
}

func RegisterExampleUnaryExtensionServer(s grpc.ServiceRegistrar, srv ExampleUnaryExtensionServer) {
	s.RegisterService(&ExampleUnaryExtension_ServiceDesc, srv)
}

func _ExampleUnaryExtension_Hello_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExampleUnaryExtensionServer).Hello(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ExampleUnaryExtension_Hello_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExampleUnaryExtensionServer).Hello(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

// ExampleUnaryExtension_ServiceDesc is the grpc.ServiceDesc for ExampleUnaryExtension service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var ExampleUnaryExtension_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "example.ExampleUnaryExtension",
	HandlerType: (*ExampleUnaryExtensionServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Hello",
			Handler:    _ExampleUnaryExtension_Hello_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "github.com/rancher/opni/plugins/example/pkg/example/example.proto",
}