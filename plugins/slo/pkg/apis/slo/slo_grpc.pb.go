// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - ragu               v1.0.0
// source: github.com/rancher/opni/pkg/plugins/slo/pkg/apis/slo/slo.proto

package slo

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

// SLOClient is the client API for SLO service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type SLOClient interface {
	// ============== SLO
	CreateSLO(ctx context.Context, in *CreateSLORequest, opts ...grpc.CallOption) (*v1.ReferenceList, error)
	GetSLO(ctx context.Context, in *v1.Reference, opts ...grpc.CallOption) (*SLOData, error)
	ListSLOs(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ServiceLevelObjectiveList, error)
	UpdateSLO(ctx context.Context, in *SLOData, opts ...grpc.CallOption) (*emptypb.Empty, error)
	DeleteSLO(ctx context.Context, in *v1.Reference, opts ...grpc.CallOption) (*emptypb.Empty, error)
	CloneSLO(ctx context.Context, in *v1.Reference, opts ...grpc.CallOption) (*SLOData, error)
	// Can this metric run with this service & cluster? No == error
	GetMetricId(ctx context.Context, in *MetricRequest, opts ...grpc.CallOption) (*Service, error)
	ListMetrics(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*MetricList, error)
	// ========== Services API ===========
	ListServices(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ServiceList, error)
	// ================ Poll SLO Status
	Status(ctx context.Context, in *v1.Reference, opts ...grpc.CallOption) (*SLOStatus, error)
}

type sLOClient struct {
	cc grpc.ClientConnInterface
}

func NewSLOClient(cc grpc.ClientConnInterface) SLOClient {
	return &sLOClient{cc}
}

func (c *sLOClient) CreateSLO(ctx context.Context, in *CreateSLORequest, opts ...grpc.CallOption) (*v1.ReferenceList, error) {
	out := new(v1.ReferenceList)
	err := c.cc.Invoke(ctx, "/slo.SLO/CreateSLO", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sLOClient) GetSLO(ctx context.Context, in *v1.Reference, opts ...grpc.CallOption) (*SLOData, error) {
	out := new(SLOData)
	err := c.cc.Invoke(ctx, "/slo.SLO/GetSLO", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sLOClient) ListSLOs(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ServiceLevelObjectiveList, error) {
	out := new(ServiceLevelObjectiveList)
	err := c.cc.Invoke(ctx, "/slo.SLO/ListSLOs", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sLOClient) UpdateSLO(ctx context.Context, in *SLOData, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/slo.SLO/UpdateSLO", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sLOClient) DeleteSLO(ctx context.Context, in *v1.Reference, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/slo.SLO/DeleteSLO", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sLOClient) CloneSLO(ctx context.Context, in *v1.Reference, opts ...grpc.CallOption) (*SLOData, error) {
	out := new(SLOData)
	err := c.cc.Invoke(ctx, "/slo.SLO/CloneSLO", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sLOClient) GetMetricId(ctx context.Context, in *MetricRequest, opts ...grpc.CallOption) (*Service, error) {
	out := new(Service)
	err := c.cc.Invoke(ctx, "/slo.SLO/GetMetricId", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sLOClient) ListMetrics(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*MetricList, error) {
	out := new(MetricList)
	err := c.cc.Invoke(ctx, "/slo.SLO/ListMetrics", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sLOClient) ListServices(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ServiceList, error) {
	out := new(ServiceList)
	err := c.cc.Invoke(ctx, "/slo.SLO/ListServices", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sLOClient) Status(ctx context.Context, in *v1.Reference, opts ...grpc.CallOption) (*SLOStatus, error) {
	out := new(SLOStatus)
	err := c.cc.Invoke(ctx, "/slo.SLO/Status", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SLOServer is the server API for SLO service.
// All implementations must embed UnimplementedSLOServer
// for forward compatibility
type SLOServer interface {
	// ============== SLO
	CreateSLO(context.Context, *CreateSLORequest) (*v1.ReferenceList, error)
	GetSLO(context.Context, *v1.Reference) (*SLOData, error)
	ListSLOs(context.Context, *emptypb.Empty) (*ServiceLevelObjectiveList, error)
	UpdateSLO(context.Context, *SLOData) (*emptypb.Empty, error)
	DeleteSLO(context.Context, *v1.Reference) (*emptypb.Empty, error)
	CloneSLO(context.Context, *v1.Reference) (*SLOData, error)
	// Can this metric run with this service & cluster? No == error
	GetMetricId(context.Context, *MetricRequest) (*Service, error)
	ListMetrics(context.Context, *emptypb.Empty) (*MetricList, error)
	// ========== Services API ===========
	ListServices(context.Context, *emptypb.Empty) (*ServiceList, error)
	// ================ Poll SLO Status
	Status(context.Context, *v1.Reference) (*SLOStatus, error)
	mustEmbedUnimplementedSLOServer()
}

// UnimplementedSLOServer must be embedded to have forward compatible implementations.
type UnimplementedSLOServer struct {
}

func (UnimplementedSLOServer) CreateSLO(context.Context, *CreateSLORequest) (*v1.ReferenceList, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateSLO not implemented")
}
func (UnimplementedSLOServer) GetSLO(context.Context, *v1.Reference) (*SLOData, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSLO not implemented")
}
func (UnimplementedSLOServer) ListSLOs(context.Context, *emptypb.Empty) (*ServiceLevelObjectiveList, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListSLOs not implemented")
}
func (UnimplementedSLOServer) UpdateSLO(context.Context, *SLOData) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateSLO not implemented")
}
func (UnimplementedSLOServer) DeleteSLO(context.Context, *v1.Reference) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteSLO not implemented")
}
func (UnimplementedSLOServer) CloneSLO(context.Context, *v1.Reference) (*SLOData, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CloneSLO not implemented")
}
func (UnimplementedSLOServer) GetMetricId(context.Context, *MetricRequest) (*Service, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetMetricId not implemented")
}
func (UnimplementedSLOServer) ListMetrics(context.Context, *emptypb.Empty) (*MetricList, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListMetrics not implemented")
}
func (UnimplementedSLOServer) ListServices(context.Context, *emptypb.Empty) (*ServiceList, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListServices not implemented")
}
func (UnimplementedSLOServer) Status(context.Context, *v1.Reference) (*SLOStatus, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedSLOServer) mustEmbedUnimplementedSLOServer() {}

// UnsafeSLOServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to SLOServer will
// result in compilation errors.
type UnsafeSLOServer interface {
	mustEmbedUnimplementedSLOServer()
}

func RegisterSLOServer(s grpc.ServiceRegistrar, srv SLOServer) {
	s.RegisterService(&SLO_ServiceDesc, srv)
}

func _SLO_CreateSLO_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateSLORequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SLOServer).CreateSLO(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/slo.SLO/CreateSLO",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SLOServer).CreateSLO(ctx, req.(*CreateSLORequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _SLO_GetSLO_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(v1.Reference)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SLOServer).GetSLO(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/slo.SLO/GetSLO",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SLOServer).GetSLO(ctx, req.(*v1.Reference))
	}
	return interceptor(ctx, in, info, handler)
}

func _SLO_ListSLOs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SLOServer).ListSLOs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/slo.SLO/ListSLOs",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SLOServer).ListSLOs(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _SLO_UpdateSLO_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SLOData)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SLOServer).UpdateSLO(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/slo.SLO/UpdateSLO",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SLOServer).UpdateSLO(ctx, req.(*SLOData))
	}
	return interceptor(ctx, in, info, handler)
}

func _SLO_DeleteSLO_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(v1.Reference)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SLOServer).DeleteSLO(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/slo.SLO/DeleteSLO",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SLOServer).DeleteSLO(ctx, req.(*v1.Reference))
	}
	return interceptor(ctx, in, info, handler)
}

func _SLO_CloneSLO_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(v1.Reference)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SLOServer).CloneSLO(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/slo.SLO/CloneSLO",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SLOServer).CloneSLO(ctx, req.(*v1.Reference))
	}
	return interceptor(ctx, in, info, handler)
}

func _SLO_GetMetricId_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MetricRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SLOServer).GetMetricId(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/slo.SLO/GetMetricId",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SLOServer).GetMetricId(ctx, req.(*MetricRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _SLO_ListMetrics_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SLOServer).ListMetrics(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/slo.SLO/ListMetrics",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SLOServer).ListMetrics(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _SLO_ListServices_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SLOServer).ListServices(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/slo.SLO/ListServices",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SLOServer).ListServices(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _SLO_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(v1.Reference)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SLOServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/slo.SLO/Status",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SLOServer).Status(ctx, req.(*v1.Reference))
	}
	return interceptor(ctx, in, info, handler)
}

// SLO_ServiceDesc is the grpc.ServiceDesc for SLO service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var SLO_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "slo.SLO",
	HandlerType: (*SLOServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CreateSLO",
			Handler:    _SLO_CreateSLO_Handler,
		},
		{
			MethodName: "GetSLO",
			Handler:    _SLO_GetSLO_Handler,
		},
		{
			MethodName: "ListSLOs",
			Handler:    _SLO_ListSLOs_Handler,
		},
		{
			MethodName: "UpdateSLO",
			Handler:    _SLO_UpdateSLO_Handler,
		},
		{
			MethodName: "DeleteSLO",
			Handler:    _SLO_DeleteSLO_Handler,
		},
		{
			MethodName: "CloneSLO",
			Handler:    _SLO_CloneSLO_Handler,
		},
		{
			MethodName: "GetMetricId",
			Handler:    _SLO_GetMetricId_Handler,
		},
		{
			MethodName: "ListMetrics",
			Handler:    _SLO_ListMetrics_Handler,
		},
		{
			MethodName: "ListServices",
			Handler:    _SLO_ListServices_Handler,
		},
		{
			MethodName: "Status",
			Handler:    _SLO_Status_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "github.com/rancher/opni/pkg/plugins/slo/pkg/apis/slo/slo.proto",
}
