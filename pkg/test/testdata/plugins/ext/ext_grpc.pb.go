// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - ragu               v1.0.0
// source: github.com/rancher/opni/pkg/test/testdata/plugins/ext/ext.proto

package ext

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// ExtClient is the client API for Ext service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ExtClient interface {
	Foo(ctx context.Context, in *FooRequest, opts ...grpc.CallOption) (*FooResponse, error)
}

type extClient struct {
	cc grpc.ClientConnInterface
}

func NewExtClient(cc grpc.ClientConnInterface) ExtClient {
	return &extClient{cc}
}

func (c *extClient) Foo(ctx context.Context, in *FooRequest, opts ...grpc.CallOption) (*FooResponse, error) {
	out := new(FooResponse)
	err := c.cc.Invoke(ctx, "/ext.Ext/Foo", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ExtServer is the server API for Ext service.
// All implementations must embed UnimplementedExtServer
// for forward compatibility
type ExtServer interface {
	Foo(context.Context, *FooRequest) (*FooResponse, error)
	mustEmbedUnimplementedExtServer()
}

// UnimplementedExtServer must be embedded to have forward compatible implementations.
type UnimplementedExtServer struct {
}

func (UnimplementedExtServer) Foo(context.Context, *FooRequest) (*FooResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Foo not implemented")
}
func (UnimplementedExtServer) mustEmbedUnimplementedExtServer() {}

// UnsafeExtServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ExtServer will
// result in compilation errors.
type UnsafeExtServer interface {
	mustEmbedUnimplementedExtServer()
}

func RegisterExtServer(s grpc.ServiceRegistrar, srv ExtServer) {
	s.RegisterService(&Ext_ServiceDesc, srv)
}

func _Ext_Foo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(FooRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExtServer).Foo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/ext.Ext/Foo",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExtServer).Foo(ctx, req.(*FooRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Ext_ServiceDesc is the grpc.ServiceDesc for Ext service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Ext_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "ext.Ext",
	HandlerType: (*ExtServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Foo",
			Handler:    _Ext_Foo_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "github.com/rancher/opni/pkg/test/testdata/plugins/ext/ext.proto",
}

// Ext2Client is the client API for Ext2 service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type Ext2Client interface {
	Foo(ctx context.Context, in *FooRequest, opts ...grpc.CallOption) (*FooResponse, error)
}

type ext2Client struct {
	cc grpc.ClientConnInterface
}

func NewExt2Client(cc grpc.ClientConnInterface) Ext2Client {
	return &ext2Client{cc}
}

func (c *ext2Client) Foo(ctx context.Context, in *FooRequest, opts ...grpc.CallOption) (*FooResponse, error) {
	out := new(FooResponse)
	err := c.cc.Invoke(ctx, "/ext.Ext2/Foo", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Ext2Server is the server API for Ext2 service.
// All implementations must embed UnimplementedExt2Server
// for forward compatibility
type Ext2Server interface {
	Foo(context.Context, *FooRequest) (*FooResponse, error)
	mustEmbedUnimplementedExt2Server()
}

// UnimplementedExt2Server must be embedded to have forward compatible implementations.
type UnimplementedExt2Server struct {
}

func (UnimplementedExt2Server) Foo(context.Context, *FooRequest) (*FooResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Foo not implemented")
}
func (UnimplementedExt2Server) mustEmbedUnimplementedExt2Server() {}

// UnsafeExt2Server may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to Ext2Server will
// result in compilation errors.
type UnsafeExt2Server interface {
	mustEmbedUnimplementedExt2Server()
}

func RegisterExt2Server(s grpc.ServiceRegistrar, srv Ext2Server) {
	s.RegisterService(&Ext2_ServiceDesc, srv)
}

func _Ext2_Foo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(FooRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(Ext2Server).Foo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/ext.Ext2/Foo",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(Ext2Server).Foo(ctx, req.(*FooRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Ext2_ServiceDesc is the grpc.ServiceDesc for Ext2 service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Ext2_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "ext.Ext2",
	HandlerType: (*Ext2Server)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Foo",
			Handler:    _Ext2_Foo_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "github.com/rancher/opni/pkg/test/testdata/plugins/ext/ext.proto",
}
