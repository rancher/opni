// Code generated by internal/codegen. DO NOT EDIT.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.30.0-devel
// 	protoc        v1.0.0
// source: github.com/rancher/opni/internal/cortex/config/alertmanager/multitenant_alertmanager.proto

package alertmanager

import (
	_ "github.com/rancher/opni/internal/cli"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type ClientConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	RemoteTimeout   *durationpb.Duration `protobuf:"bytes,1,opt,name=remote_timeout,json=remoteTimeout,proto3" json:"remote_timeout,omitempty"`
	TlsEnabled      bool                 `protobuf:"varint,2,opt,name=tls_enabled,json=tlsEnabled,proto3" json:"tls_enabled,omitempty"`
	TLS             *TlsClientConfig     `protobuf:"bytes,3,opt,name=TLS,proto3" json:"TLS,omitempty"`
	GrpcCompression string               `protobuf:"bytes,4,opt,name=grpc_compression,json=grpcCompression,proto3" json:"grpc_compression,omitempty"`
}

func (x *ClientConfig) Reset() {
	*x = ClientConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ClientConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClientConfig) ProtoMessage() {}

func (x *ClientConfig) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClientConfig.ProtoReflect.Descriptor instead.
func (*ClientConfig) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescGZIP(), []int{0}
}

func (x *ClientConfig) GetRemoteTimeout() *durationpb.Duration {
	if x != nil {
		return x.RemoteTimeout
	}
	return nil
}

func (x *ClientConfig) GetTlsEnabled() bool {
	if x != nil {
		return x.TlsEnabled
	}
	return false
}

func (x *ClientConfig) GetTLS() *TlsClientConfig {
	if x != nil {
		return x.TLS
	}
	return nil
}

func (x *ClientConfig) GetGrpcCompression() string {
	if x != nil {
		return x.GrpcCompression
	}
	return ""
}

type ClusterConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Time to wait between peers to send notifications.
	PeerTimeout *durationpb.Duration `protobuf:"bytes,1,opt,name=peer_timeout,json=peerTimeout,proto3" json:"peer_timeout,omitempty"`
	// The interval between sending gossip messages. By lowering this value (more frequent) gossip messages are propagated across cluster more quickly at the expense of increased bandwidth usage.
	GossipInterval *durationpb.Duration `protobuf:"bytes,2,opt,name=gossip_interval,json=gossipInterval,proto3" json:"gossip_interval,omitempty"`
	// The interval between gossip state syncs. Setting this interval lower (more frequent) will increase convergence speeds across larger clusters at the expense of increased bandwidth usage.
	PushPullInterval *durationpb.Duration `protobuf:"bytes,3,opt,name=push_pull_interval,json=pushPullInterval,proto3" json:"push_pull_interval,omitempty"`
}

func (x *ClusterConfig) Reset() {
	*x = ClusterConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ClusterConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterConfig) ProtoMessage() {}

func (x *ClusterConfig) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterConfig.ProtoReflect.Descriptor instead.
func (*ClusterConfig) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescGZIP(), []int{1}
}

func (x *ClusterConfig) GetPeerTimeout() *durationpb.Duration {
	if x != nil {
		return x.PeerTimeout
	}
	return nil
}

func (x *ClusterConfig) GetGossipInterval() *durationpb.Duration {
	if x != nil {
		return x.GossipInterval
	}
	return nil
}

func (x *ClusterConfig) GetPushPullInterval() *durationpb.Duration {
	if x != nil {
		return x.PushPullInterval
	}
	return nil
}

type MultitenantAlertmanagerConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// How long to keep data for.
	Retention *durationpb.Duration `protobuf:"bytes,1,opt,name=retention,proto3" json:"retention,omitempty"`
	// The URL under which Alertmanager is externally reachable (for example, if Alertmanager is served via a reverse proxy). Used for generating relative and absolute links back to Alertmanager itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Alertmanager. If omitted, relevant URL components will be derived automatically.
	ExternalUrl string `protobuf:"bytes,2,opt,name=external_url,json=externalUrl,proto3" json:"external_url,omitempty"`
	// How frequently to poll Cortex configs
	PollInterval *durationpb.Duration `protobuf:"bytes,3,opt,name=poll_interval,json=pollInterval,proto3" json:"poll_interval,omitempty"`
	// Maximum size (bytes) of an accepted HTTP request body.
	MaxRecvMsgSize int64 `protobuf:"varint,4,opt,name=max_recv_msg_size,json=maxRecvMsgSize,proto3" json:"max_recv_msg_size,omitempty"`
	// Root of URL to generate if config is http://internal.monitor
	AutoWebhookRoot string `protobuf:"bytes,5,opt,name=auto_webhook_root,json=autoWebhookRoot,proto3" json:"auto_webhook_root,omitempty"`
	// Listen address and port for the cluster. Not specifying this flag disables high-availability mode.
	Cluster *ClusterConfig `protobuf:"bytes,6,opt,name=cluster,proto3" json:"cluster,omitempty"`
	// Timeout for downstream alertmanagers.
	AlertmanagerClient *ClientConfig `protobuf:"bytes,7,opt,name=alertmanager_client,json=alertmanagerClient,proto3" json:"alertmanager_client,omitempty"`
	// The interval between persisting the current alertmanager state (notification log and silences) to object storage. This is only used when sharding is enabled. This state is read when all replicas for a shard can not be contacted. In this scenario, having persisted the state more frequently will result in potentially fewer lost silences, and fewer duplicate notifications.
	Persister *PersisterConfig `protobuf:"bytes,8,opt,name=Persister,proto3" json:"Persister,omitempty"`
	// Comma separated list of tenants whose alerts this alertmanager can process. If specified, only these tenants will be handled by alertmanager, otherwise this alertmanager can process alerts from all tenants.
	EnabledTenants []string `protobuf:"bytes,9,rep,name=enabled_tenants,json=enabledTenants,proto3" json:"enabled_tenants,omitempty"`
	// Comma separated list of tenants whose alerts this alertmanager cannot process. If specified, a alertmanager that would normally pick the specified tenant(s) for processing will ignore them instead.
	DisabledTenants []string `protobuf:"bytes,10,rep,name=disabled_tenants,json=disabledTenants,proto3" json:"disabled_tenants,omitempty"`
}

func (x *MultitenantAlertmanagerConfig) Reset() {
	*x = MultitenantAlertmanagerConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MultitenantAlertmanagerConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MultitenantAlertmanagerConfig) ProtoMessage() {}

func (x *MultitenantAlertmanagerConfig) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MultitenantAlertmanagerConfig.ProtoReflect.Descriptor instead.
func (*MultitenantAlertmanagerConfig) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescGZIP(), []int{2}
}

func (x *MultitenantAlertmanagerConfig) GetRetention() *durationpb.Duration {
	if x != nil {
		return x.Retention
	}
	return nil
}

func (x *MultitenantAlertmanagerConfig) GetExternalUrl() string {
	if x != nil {
		return x.ExternalUrl
	}
	return ""
}

func (x *MultitenantAlertmanagerConfig) GetPollInterval() *durationpb.Duration {
	if x != nil {
		return x.PollInterval
	}
	return nil
}

func (x *MultitenantAlertmanagerConfig) GetMaxRecvMsgSize() int64 {
	if x != nil {
		return x.MaxRecvMsgSize
	}
	return 0
}

func (x *MultitenantAlertmanagerConfig) GetAutoWebhookRoot() string {
	if x != nil {
		return x.AutoWebhookRoot
	}
	return ""
}

func (x *MultitenantAlertmanagerConfig) GetCluster() *ClusterConfig {
	if x != nil {
		return x.Cluster
	}
	return nil
}

func (x *MultitenantAlertmanagerConfig) GetAlertmanagerClient() *ClientConfig {
	if x != nil {
		return x.AlertmanagerClient
	}
	return nil
}

func (x *MultitenantAlertmanagerConfig) GetPersister() *PersisterConfig {
	if x != nil {
		return x.Persister
	}
	return nil
}

func (x *MultitenantAlertmanagerConfig) GetEnabledTenants() []string {
	if x != nil {
		return x.EnabledTenants
	}
	return nil
}

func (x *MultitenantAlertmanagerConfig) GetDisabledTenants() []string {
	if x != nil {
		return x.DisabledTenants
	}
	return nil
}

type PersisterConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	PersistInterval *durationpb.Duration `protobuf:"bytes,1,opt,name=persist_interval,json=persistInterval,proto3" json:"persist_interval,omitempty"`
}

func (x *PersisterConfig) Reset() {
	*x = PersisterConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PersisterConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PersisterConfig) ProtoMessage() {}

func (x *PersisterConfig) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PersisterConfig.ProtoReflect.Descriptor instead.
func (*PersisterConfig) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescGZIP(), []int{3}
}

func (x *PersisterConfig) GetPersistInterval() *durationpb.Duration {
	if x != nil {
		return x.PersistInterval
	}
	return nil
}

type TlsClientConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	TlsCertPath           string `protobuf:"bytes,1,opt,name=tls_cert_path,json=tlsCertPath,proto3" json:"tls_cert_path,omitempty"`
	TlsKeyPath            string `protobuf:"bytes,2,opt,name=tls_key_path,json=tlsKeyPath,proto3" json:"tls_key_path,omitempty"`
	TlsCaPath             string `protobuf:"bytes,3,opt,name=tls_ca_path,json=tlsCaPath,proto3" json:"tls_ca_path,omitempty"`
	TlsServerName         string `protobuf:"bytes,4,opt,name=tls_server_name,json=tlsServerName,proto3" json:"tls_server_name,omitempty"`
	TlsInsecureSkipVerify bool   `protobuf:"varint,5,opt,name=tls_insecure_skip_verify,json=tlsInsecureSkipVerify,proto3" json:"tls_insecure_skip_verify,omitempty"`
}

func (x *TlsClientConfig) Reset() {
	*x = TlsClientConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TlsClientConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TlsClientConfig) ProtoMessage() {}

func (x *TlsClientConfig) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TlsClientConfig.ProtoReflect.Descriptor instead.
func (*TlsClientConfig) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescGZIP(), []int{4}
}

func (x *TlsClientConfig) GetTlsCertPath() string {
	if x != nil {
		return x.TlsCertPath
	}
	return ""
}

func (x *TlsClientConfig) GetTlsKeyPath() string {
	if x != nil {
		return x.TlsKeyPath
	}
	return ""
}

func (x *TlsClientConfig) GetTlsCaPath() string {
	if x != nil {
		return x.TlsCaPath
	}
	return ""
}

func (x *TlsClientConfig) GetTlsServerName() string {
	if x != nil {
		return x.TlsServerName
	}
	return ""
}

func (x *TlsClientConfig) GetTlsInsecureSkipVerify() bool {
	if x != nil {
		return x.TlsInsecureSkipVerify
	}
	return false
}

var File_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto protoreflect.FileDescriptor

var file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDesc = []byte{
	0x0a, 0x5a, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e,
	0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x2f, 0x63, 0x6f, 0x72, 0x74, 0x65, 0x78, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x2f, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x2f, 0x6d, 0x75,
	0x6c, 0x74, 0x69, 0x74, 0x65, 0x6e, 0x61, 0x6e, 0x74, 0x5f, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x6d,
	0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0c, 0x61, 0x6c,
	0x65, 0x72, 0x74, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x1a, 0x36, 0x67, 0x69, 0x74, 0x68,
	0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f,
	0x70, 0x6e, 0x69, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x63, 0x6f, 0x64,
	0x65, 0x67, 0x65, 0x6e, 0x2f, 0x63, 0x6c, 0x69, 0x2f, 0x63, 0x6c, 0x69, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x1a, 0x1e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2f, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x22, 0xcd, 0x01, 0x0a, 0x0c, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x43, 0x6f, 0x6e,
	0x66, 0x69, 0x67, 0x12, 0x40, 0x0a, 0x0e, 0x72, 0x65, 0x6d, 0x6f, 0x74, 0x65, 0x5f, 0x74, 0x69,
	0x6d, 0x65, 0x6f, 0x75, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75,
	0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x0d, 0x72, 0x65, 0x6d, 0x6f, 0x74, 0x65, 0x54, 0x69,
	0x6d, 0x65, 0x6f, 0x75, 0x74, 0x12, 0x1f, 0x0a, 0x0b, 0x74, 0x6c, 0x73, 0x5f, 0x65, 0x6e, 0x61,
	0x62, 0x6c, 0x65, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0a, 0x74, 0x6c, 0x73, 0x45,
	0x6e, 0x61, 0x62, 0x6c, 0x65, 0x64, 0x12, 0x2f, 0x0a, 0x03, 0x54, 0x4c, 0x53, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x6d, 0x61, 0x6e, 0x61, 0x67,
	0x65, 0x72, 0x2e, 0x54, 0x6c, 0x73, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x43, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x52, 0x03, 0x54, 0x4c, 0x53, 0x12, 0x29, 0x0a, 0x10, 0x67, 0x72, 0x70, 0x63, 0x5f,
	0x63, 0x6f, 0x6d, 0x70, 0x72, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0f, 0x67, 0x72, 0x70, 0x63, 0x43, 0x6f, 0x6d, 0x70, 0x72, 0x65, 0x73, 0x73, 0x69,
	0x6f, 0x6e, 0x22, 0xfe, 0x01, 0x0a, 0x0d, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x43, 0x6f,
	0x6e, 0x66, 0x69, 0x67, 0x12, 0x47, 0x0a, 0x0c, 0x70, 0x65, 0x65, 0x72, 0x5f, 0x74, 0x69, 0x6d,
	0x65, 0x6f, 0x75, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x42, 0x09, 0x8a, 0xc0, 0x0c, 0x05, 0x0a, 0x03, 0x31, 0x35, 0x73,
	0x52, 0x0b, 0x70, 0x65, 0x65, 0x72, 0x54, 0x69, 0x6d, 0x65, 0x6f, 0x75, 0x74, 0x12, 0x4f, 0x0a,
	0x0f, 0x67, 0x6f, 0x73, 0x73, 0x69, 0x70, 0x5f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x76, 0x61, 0x6c,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x42, 0x0b, 0x8a, 0xc0, 0x0c, 0x07, 0x0a, 0x05, 0x32, 0x30, 0x30, 0x6d, 0x73, 0x52, 0x0e,
	0x67, 0x6f, 0x73, 0x73, 0x69, 0x70, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x76, 0x61, 0x6c, 0x12, 0x53,
	0x0a, 0x12, 0x70, 0x75, 0x73, 0x68, 0x5f, 0x70, 0x75, 0x6c, 0x6c, 0x5f, 0x69, 0x6e, 0x74, 0x65,
	0x72, 0x76, 0x61, 0x6c, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x42, 0x0a, 0x8a, 0xc0, 0x0c, 0x06, 0x0a, 0x04, 0x31, 0x6d, 0x30,
	0x73, 0x52, 0x10, 0x70, 0x75, 0x73, 0x68, 0x50, 0x75, 0x6c, 0x6c, 0x49, 0x6e, 0x74, 0x65, 0x72,
	0x76, 0x61, 0x6c, 0x22, 0xfd, 0x04, 0x0a, 0x1d, 0x4d, 0x75, 0x6c, 0x74, 0x69, 0x74, 0x65, 0x6e,
	0x61, 0x6e, 0x74, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x43,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x47, 0x0a, 0x09, 0x72, 0x65, 0x74, 0x65, 0x6e, 0x74, 0x69,
	0x6f, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c,
	0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x42, 0x0e, 0x8a, 0xc0, 0x0c, 0x0a, 0x0a, 0x08, 0x31, 0x32, 0x30, 0x68, 0x30,
	0x6d, 0x30, 0x73, 0x52, 0x09, 0x72, 0x65, 0x74, 0x65, 0x6e, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x21,
	0x0a, 0x0c, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x5f, 0x75, 0x72, 0x6c, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x55, 0x72,
	0x6c, 0x12, 0x49, 0x0a, 0x0d, 0x70, 0x6f, 0x6c, 0x6c, 0x5f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x76,
	0x61, 0x6c, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c,
	0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x42, 0x09, 0x8a, 0xc0, 0x0c, 0x05, 0x0a, 0x03, 0x31, 0x35, 0x73, 0x52, 0x0c,
	0x70, 0x6f, 0x6c, 0x6c, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x76, 0x61, 0x6c, 0x12, 0x39, 0x0a, 0x11,
	0x6d, 0x61, 0x78, 0x5f, 0x72, 0x65, 0x63, 0x76, 0x5f, 0x6d, 0x73, 0x67, 0x5f, 0x73, 0x69, 0x7a,
	0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x03, 0x42, 0x0e, 0x8a, 0xc0, 0x0c, 0x0a, 0x0a, 0x08, 0x31,
	0x36, 0x37, 0x37, 0x37, 0x32, 0x31, 0x36, 0x52, 0x0e, 0x6d, 0x61, 0x78, 0x52, 0x65, 0x63, 0x76,
	0x4d, 0x73, 0x67, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x2a, 0x0a, 0x11, 0x61, 0x75, 0x74, 0x6f, 0x5f,
	0x77, 0x65, 0x62, 0x68, 0x6f, 0x6f, 0x6b, 0x5f, 0x72, 0x6f, 0x6f, 0x74, 0x18, 0x05, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0f, 0x61, 0x75, 0x74, 0x6f, 0x57, 0x65, 0x62, 0x68, 0x6f, 0x6f, 0x6b, 0x52,
	0x6f, 0x6f, 0x74, 0x12, 0x49, 0x0a, 0x07, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x18, 0x06,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x6d, 0x61, 0x6e, 0x61,
	0x67, 0x65, 0x72, 0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x43, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x42, 0x12, 0x8a, 0xc0, 0x0c, 0x0e, 0x0a, 0x0c, 0x30, 0x2e, 0x30, 0x2e, 0x30, 0x2e, 0x30,
	0x3a, 0x39, 0x30, 0x39, 0x34, 0x52, 0x07, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x12, 0x55,
	0x0a, 0x13, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x5f, 0x63,
	0x6c, 0x69, 0x65, 0x6e, 0x74, 0x18, 0x07, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x61, 0x6c,
	0x65, 0x72, 0x74, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x2e, 0x43, 0x6c, 0x69, 0x65, 0x6e,
	0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x42, 0x08, 0x8a, 0xc0, 0x0c, 0x04, 0x0a, 0x02, 0x32,
	0x73, 0x52, 0x12, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x43,
	0x6c, 0x69, 0x65, 0x6e, 0x74, 0x12, 0x48, 0x0a, 0x09, 0x50, 0x65, 0x72, 0x73, 0x69, 0x73, 0x74,
	0x65, 0x72, 0x18, 0x08, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x61, 0x6c, 0x65, 0x72, 0x74,
	0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x2e, 0x50, 0x65, 0x72, 0x73, 0x69, 0x73, 0x74, 0x65,
	0x72, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x42, 0x0b, 0x8a, 0xc0, 0x0c, 0x07, 0x0a, 0x05, 0x31,
	0x35, 0x6d, 0x30, 0x73, 0x52, 0x09, 0x50, 0x65, 0x72, 0x73, 0x69, 0x73, 0x74, 0x65, 0x72, 0x12,
	0x27, 0x0a, 0x0f, 0x65, 0x6e, 0x61, 0x62, 0x6c, 0x65, 0x64, 0x5f, 0x74, 0x65, 0x6e, 0x61, 0x6e,
	0x74, 0x73, 0x18, 0x09, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0e, 0x65, 0x6e, 0x61, 0x62, 0x6c, 0x65,
	0x64, 0x54, 0x65, 0x6e, 0x61, 0x6e, 0x74, 0x73, 0x12, 0x29, 0x0a, 0x10, 0x64, 0x69, 0x73, 0x61,
	0x62, 0x6c, 0x65, 0x64, 0x5f, 0x74, 0x65, 0x6e, 0x61, 0x6e, 0x74, 0x73, 0x18, 0x0a, 0x20, 0x03,
	0x28, 0x09, 0x52, 0x0f, 0x64, 0x69, 0x73, 0x61, 0x62, 0x6c, 0x65, 0x64, 0x54, 0x65, 0x6e, 0x61,
	0x6e, 0x74, 0x73, 0x22, 0x57, 0x0a, 0x0f, 0x50, 0x65, 0x72, 0x73, 0x69, 0x73, 0x74, 0x65, 0x72,
	0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x44, 0x0a, 0x10, 0x70, 0x65, 0x72, 0x73, 0x69, 0x73,
	0x74, 0x5f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x76, 0x61, 0x6c, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x0f, 0x70, 0x65, 0x72,
	0x73, 0x69, 0x73, 0x74, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x76, 0x61, 0x6c, 0x22, 0xd8, 0x01, 0x0a,
	0x0f, 0x54, 0x6c, 0x73, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x12, 0x22, 0x0a, 0x0d, 0x74, 0x6c, 0x73, 0x5f, 0x63, 0x65, 0x72, 0x74, 0x5f, 0x70, 0x61, 0x74,
	0x68, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x74, 0x6c, 0x73, 0x43, 0x65, 0x72, 0x74,
	0x50, 0x61, 0x74, 0x68, 0x12, 0x20, 0x0a, 0x0c, 0x74, 0x6c, 0x73, 0x5f, 0x6b, 0x65, 0x79, 0x5f,
	0x70, 0x61, 0x74, 0x68, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x74, 0x6c, 0x73, 0x4b,
	0x65, 0x79, 0x50, 0x61, 0x74, 0x68, 0x12, 0x1e, 0x0a, 0x0b, 0x74, 0x6c, 0x73, 0x5f, 0x63, 0x61,
	0x5f, 0x70, 0x61, 0x74, 0x68, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x74, 0x6c, 0x73,
	0x43, 0x61, 0x50, 0x61, 0x74, 0x68, 0x12, 0x26, 0x0a, 0x0f, 0x74, 0x6c, 0x73, 0x5f, 0x73, 0x65,
	0x72, 0x76, 0x65, 0x72, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0d, 0x74, 0x6c, 0x73, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x37,
	0x0a, 0x18, 0x74, 0x6c, 0x73, 0x5f, 0x69, 0x6e, 0x73, 0x65, 0x63, 0x75, 0x72, 0x65, 0x5f, 0x73,
	0x6b, 0x69, 0x70, 0x5f, 0x76, 0x65, 0x72, 0x69, 0x66, 0x79, 0x18, 0x05, 0x20, 0x01, 0x28, 0x08,
	0x52, 0x15, 0x74, 0x6c, 0x73, 0x49, 0x6e, 0x73, 0x65, 0x63, 0x75, 0x72, 0x65, 0x53, 0x6b, 0x69,
	0x70, 0x56, 0x65, 0x72, 0x69, 0x66, 0x79, 0x42, 0x45, 0x82, 0xc0, 0x0c, 0x04, 0x08, 0x01, 0x10,
	0x01, 0x5a, 0x3b, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61,
	0x6e, 0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72,
	0x6e, 0x61, 0x6c, 0x2f, 0x63, 0x6f, 0x72, 0x74, 0x65, 0x78, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x2f, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x62, 0x06,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescOnce sync.Once
	file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescData = file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDesc
)

func file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescGZIP() []byte {
	file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescOnce.Do(func() {
		file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescData = protoimpl.X.CompressGZIP(file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescData)
	})
	return file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDescData
}

var file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_goTypes = []interface{}{
	(*ClientConfig)(nil),                  // 0: alertmanager.ClientConfig
	(*ClusterConfig)(nil),                 // 1: alertmanager.ClusterConfig
	(*MultitenantAlertmanagerConfig)(nil), // 2: alertmanager.MultitenantAlertmanagerConfig
	(*PersisterConfig)(nil),               // 3: alertmanager.PersisterConfig
	(*TlsClientConfig)(nil),               // 4: alertmanager.TlsClientConfig
	(*durationpb.Duration)(nil),           // 5: google.protobuf.Duration
}
var file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_depIdxs = []int32{
	5,  // 0: alertmanager.ClientConfig.remote_timeout:type_name -> google.protobuf.Duration
	4,  // 1: alertmanager.ClientConfig.TLS:type_name -> alertmanager.TlsClientConfig
	5,  // 2: alertmanager.ClusterConfig.peer_timeout:type_name -> google.protobuf.Duration
	5,  // 3: alertmanager.ClusterConfig.gossip_interval:type_name -> google.protobuf.Duration
	5,  // 4: alertmanager.ClusterConfig.push_pull_interval:type_name -> google.protobuf.Duration
	5,  // 5: alertmanager.MultitenantAlertmanagerConfig.retention:type_name -> google.protobuf.Duration
	5,  // 6: alertmanager.MultitenantAlertmanagerConfig.poll_interval:type_name -> google.protobuf.Duration
	1,  // 7: alertmanager.MultitenantAlertmanagerConfig.cluster:type_name -> alertmanager.ClusterConfig
	0,  // 8: alertmanager.MultitenantAlertmanagerConfig.alertmanager_client:type_name -> alertmanager.ClientConfig
	3,  // 9: alertmanager.MultitenantAlertmanagerConfig.Persister:type_name -> alertmanager.PersisterConfig
	5,  // 10: alertmanager.PersisterConfig.persist_interval:type_name -> google.protobuf.Duration
	11, // [11:11] is the sub-list for method output_type
	11, // [11:11] is the sub-list for method input_type
	11, // [11:11] is the sub-list for extension type_name
	11, // [11:11] is the sub-list for extension extendee
	0,  // [0:11] is the sub-list for field type_name
}

func init() {
	file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_init()
}
func file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_init() {
	if File_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ClientConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ClusterConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MultitenantAlertmanagerConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PersisterConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TlsClientConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_goTypes,
		DependencyIndexes: file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_depIdxs,
		MessageInfos:      file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_msgTypes,
	}.Build()
	File_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto = out.File
	file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_rawDesc = nil
	file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_goTypes = nil
	file_github_com_rancher_opni_internal_cortex_config_alertmanager_multitenant_alertmanager_proto_depIdxs = nil
}
