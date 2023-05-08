// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - ragu               v1.0.0
// source: github.com/rancher/opni/plugins/alerting/pkg/apis/alertops/v2/alertops.proto

package v2

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
	AlertingAdminV2_GetClusterConfiguration_FullMethodName = "/alerting.ops.v2.AlertingAdminV2/GetClusterConfiguration"
	AlertingAdminV2_ConfigureCluster_FullMethodName        = "/alerting.ops.v2.AlertingAdminV2/ConfigureCluster"
	AlertingAdminV2_GetClusterStatus_FullMethodName        = "/alerting.ops.v2.AlertingAdminV2/GetClusterStatus"
	AlertingAdminV2_UninstallCluster_FullMethodName        = "/alerting.ops.v2.AlertingAdminV2/UninstallCluster"
)

// AlertingAdminV2Client is the client API for AlertingAdminV2 service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type AlertingAdminV2Client interface {
	GetClusterConfiguration(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ClusterConfiguration, error)
	// Install/Uninstall the alerting cluster by setting enabled=true/false
	ConfigureCluster(ctx context.Context, in *ClusterConfiguration, opts ...grpc.CallOption) (*emptypb.Empty, error)
	GetClusterStatus(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*InstallStatus, error)
	UninstallCluster(ctx context.Context, in *UninstallRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type alertingAdminV2Client struct {
	cc grpc.ClientConnInterface
}

func NewAlertingAdminV2Client(cc grpc.ClientConnInterface) AlertingAdminV2Client {
	return &alertingAdminV2Client{cc}
}

func (c *alertingAdminV2Client) GetClusterConfiguration(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ClusterConfiguration, error) {
	out := new(ClusterConfiguration)
	err := c.cc.Invoke(ctx, AlertingAdminV2_GetClusterConfiguration_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *alertingAdminV2Client) ConfigureCluster(ctx context.Context, in *ClusterConfiguration, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, AlertingAdminV2_ConfigureCluster_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *alertingAdminV2Client) GetClusterStatus(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*InstallStatus, error) {
	out := new(InstallStatus)
	err := c.cc.Invoke(ctx, AlertingAdminV2_GetClusterStatus_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *alertingAdminV2Client) UninstallCluster(ctx context.Context, in *UninstallRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, AlertingAdminV2_UninstallCluster_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AlertingAdminV2Server is the server API for AlertingAdminV2 service.
// All implementations must embed UnimplementedAlertingAdminV2Server
// for forward compatibility
type AlertingAdminV2Server interface {
	GetClusterConfiguration(context.Context, *emptypb.Empty) (*ClusterConfiguration, error)
	// Install/Uninstall the alerting cluster by setting enabled=true/false
	ConfigureCluster(context.Context, *ClusterConfiguration) (*emptypb.Empty, error)
	GetClusterStatus(context.Context, *emptypb.Empty) (*InstallStatus, error)
	UninstallCluster(context.Context, *UninstallRequest) (*emptypb.Empty, error)
	mustEmbedUnimplementedAlertingAdminV2Server()
}

// UnimplementedAlertingAdminV2Server must be embedded to have forward compatible implementations.
type UnimplementedAlertingAdminV2Server struct {
}

func (UnimplementedAlertingAdminV2Server) GetClusterConfiguration(context.Context, *emptypb.Empty) (*ClusterConfiguration, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetClusterConfiguration not implemented")
}
func (UnimplementedAlertingAdminV2Server) ConfigureCluster(context.Context, *ClusterConfiguration) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ConfigureCluster not implemented")
}
func (UnimplementedAlertingAdminV2Server) GetClusterStatus(context.Context, *emptypb.Empty) (*InstallStatus, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetClusterStatus not implemented")
}
func (UnimplementedAlertingAdminV2Server) UninstallCluster(context.Context, *UninstallRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UninstallCluster not implemented")
}
func (UnimplementedAlertingAdminV2Server) mustEmbedUnimplementedAlertingAdminV2Server() {}

// UnsafeAlertingAdminV2Server may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to AlertingAdminV2Server will
// result in compilation errors.
type UnsafeAlertingAdminV2Server interface {
	mustEmbedUnimplementedAlertingAdminV2Server()
}

func RegisterAlertingAdminV2Server(s grpc.ServiceRegistrar, srv AlertingAdminV2Server) {
	s.RegisterService(&AlertingAdminV2_ServiceDesc, srv)
}

func _AlertingAdminV2_GetClusterConfiguration_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AlertingAdminV2Server).GetClusterConfiguration(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AlertingAdminV2_GetClusterConfiguration_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AlertingAdminV2Server).GetClusterConfiguration(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _AlertingAdminV2_ConfigureCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ClusterConfiguration)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AlertingAdminV2Server).ConfigureCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AlertingAdminV2_ConfigureCluster_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AlertingAdminV2Server).ConfigureCluster(ctx, req.(*ClusterConfiguration))
	}
	return interceptor(ctx, in, info, handler)
}

func _AlertingAdminV2_GetClusterStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AlertingAdminV2Server).GetClusterStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AlertingAdminV2_GetClusterStatus_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AlertingAdminV2Server).GetClusterStatus(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _AlertingAdminV2_UninstallCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UninstallRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AlertingAdminV2Server).UninstallCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AlertingAdminV2_UninstallCluster_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AlertingAdminV2Server).UninstallCluster(ctx, req.(*UninstallRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// AlertingAdminV2_ServiceDesc is the grpc.ServiceDesc for AlertingAdminV2 service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var AlertingAdminV2_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "alerting.ops.v2.AlertingAdminV2",
	HandlerType: (*AlertingAdminV2Server)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetClusterConfiguration",
			Handler:    _AlertingAdminV2_GetClusterConfiguration_Handler,
		},
		{
			MethodName: "ConfigureCluster",
			Handler:    _AlertingAdminV2_ConfigureCluster_Handler,
		},
		{
			MethodName: "GetClusterStatus",
			Handler:    _AlertingAdminV2_GetClusterStatus_Handler,
		},
		{
			MethodName: "UninstallCluster",
			Handler:    _AlertingAdminV2_UninstallCluster_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "github.com/rancher/opni/plugins/alerting/pkg/apis/alertops/v2/alertops.proto",
}

const (
	ConfigReconciler_ConnectRemoteSyncer_FullMethodName = "/alerting.ops.v2.ConfigReconciler/ConnectRemoteSyncer"
)

// ConfigReconcilerClient is the client API for ConfigReconciler service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ConfigReconcilerClient interface {
	ConnectRemoteSyncer(ctx context.Context, in *ConnectRequest, opts ...grpc.CallOption) (ConfigReconciler_ConnectRemoteSyncerClient, error)
}

type configReconcilerClient struct {
	cc grpc.ClientConnInterface
}

func NewConfigReconcilerClient(cc grpc.ClientConnInterface) ConfigReconcilerClient {
	return &configReconcilerClient{cc}
}

func (c *configReconcilerClient) ConnectRemoteSyncer(ctx context.Context, in *ConnectRequest, opts ...grpc.CallOption) (ConfigReconciler_ConnectRemoteSyncerClient, error) {
	stream, err := c.cc.NewStream(ctx, &ConfigReconciler_ServiceDesc.Streams[0], ConfigReconciler_ConnectRemoteSyncer_FullMethodName, opts...)
	if err != nil {
		return nil, err
	}
	x := &configReconcilerConnectRemoteSyncerClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type ConfigReconciler_ConnectRemoteSyncerClient interface {
	Recv() (*SyncRequest, error)
	grpc.ClientStream
}

type configReconcilerConnectRemoteSyncerClient struct {
	grpc.ClientStream
}

func (x *configReconcilerConnectRemoteSyncerClient) Recv() (*SyncRequest, error) {
	m := new(SyncRequest)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// ConfigReconcilerServer is the server API for ConfigReconciler service.
// All implementations must embed UnimplementedConfigReconcilerServer
// for forward compatibility
type ConfigReconcilerServer interface {
	ConnectRemoteSyncer(*ConnectRequest, ConfigReconciler_ConnectRemoteSyncerServer) error
	mustEmbedUnimplementedConfigReconcilerServer()
}

// UnimplementedConfigReconcilerServer must be embedded to have forward compatible implementations.
type UnimplementedConfigReconcilerServer struct {
}

func (UnimplementedConfigReconcilerServer) ConnectRemoteSyncer(*ConnectRequest, ConfigReconciler_ConnectRemoteSyncerServer) error {
	return status.Errorf(codes.Unimplemented, "method ConnectRemoteSyncer not implemented")
}
func (UnimplementedConfigReconcilerServer) mustEmbedUnimplementedConfigReconcilerServer() {}

// UnsafeConfigReconcilerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ConfigReconcilerServer will
// result in compilation errors.
type UnsafeConfigReconcilerServer interface {
	mustEmbedUnimplementedConfigReconcilerServer()
}

func RegisterConfigReconcilerServer(s grpc.ServiceRegistrar, srv ConfigReconcilerServer) {
	s.RegisterService(&ConfigReconciler_ServiceDesc, srv)
}

func _ConfigReconciler_ConnectRemoteSyncer_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(ConnectRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(ConfigReconcilerServer).ConnectRemoteSyncer(m, &configReconcilerConnectRemoteSyncerServer{stream})
}

type ConfigReconciler_ConnectRemoteSyncerServer interface {
	Send(*SyncRequest) error
	grpc.ServerStream
}

type configReconcilerConnectRemoteSyncerServer struct {
	grpc.ServerStream
}

func (x *configReconcilerConnectRemoteSyncerServer) Send(m *SyncRequest) error {
	return x.ServerStream.SendMsg(m)
}

// ConfigReconciler_ServiceDesc is the grpc.ServiceDesc for ConfigReconciler service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var ConfigReconciler_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "alerting.ops.v2.ConfigReconciler",
	HandlerType: (*ConfigReconcilerServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ConnectRemoteSyncer",
			Handler:       _ConfigReconciler_ConnectRemoteSyncer_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "github.com/rancher/opni/plugins/alerting/pkg/apis/alertops/v2/alertops.proto",
}
