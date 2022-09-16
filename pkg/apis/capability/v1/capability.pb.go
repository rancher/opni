// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1-devel
// 	protoc        v1.0.0
// source: github.com/rancher/opni/pkg/apis/capability/v1/capability.proto

package v1

import (
	_ "github.com/kralicky/totem"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	_ "google.golang.org/protobuf/types/known/durationpb"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type SyncRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Cluster *v1.Reference `protobuf:"bytes,1,opt,name=cluster,proto3" json:"cluster,omitempty"`
	Filter  *Filter       `protobuf:"bytes,2,opt,name=filter,proto3" json:"filter,omitempty"`
}

func (x *SyncRequest) Reset() {
	*x = SyncRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SyncRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SyncRequest) ProtoMessage() {}

func (x *SyncRequest) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SyncRequest.ProtoReflect.Descriptor instead.
func (*SyncRequest) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescGZIP(), []int{0}
}

func (x *SyncRequest) GetCluster() *v1.Reference {
	if x != nil {
		return x.Cluster
	}
	return nil
}

func (x *SyncRequest) GetFilter() *Filter {
	if x != nil {
		return x.Filter
	}
	return nil
}

type Filter struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	CapabilityNames []string `protobuf:"bytes,1,rep,name=capabilityNames,proto3" json:"capabilityNames,omitempty"`
}

func (x *Filter) Reset() {
	*x = Filter{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Filter) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Filter) ProtoMessage() {}

func (x *Filter) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Filter.ProtoReflect.Descriptor instead.
func (*Filter) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescGZIP(), []int{1}
}

func (x *Filter) GetCapabilityNames() []string {
	if x != nil {
		return x.CapabilityNames
	}
	return nil
}

type InfoResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	CapabilityName string `protobuf:"bytes,1,opt,name=capabilityName,proto3" json:"capabilityName,omitempty"`
}

func (x *InfoResponse) Reset() {
	*x = InfoResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *InfoResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InfoResponse) ProtoMessage() {}

func (x *InfoResponse) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InfoResponse.ProtoReflect.Descriptor instead.
func (*InfoResponse) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescGZIP(), []int{2}
}

func (x *InfoResponse) GetCapabilityName() string {
	if x != nil {
		return x.CapabilityName
	}
	return ""
}

type InstallRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Cluster *v1.Reference `protobuf:"bytes,1,opt,name=cluster,proto3" json:"cluster,omitempty"`
}

func (x *InstallRequest) Reset() {
	*x = InstallRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *InstallRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InstallRequest) ProtoMessage() {}

func (x *InstallRequest) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InstallRequest.ProtoReflect.Descriptor instead.
func (*InstallRequest) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescGZIP(), []int{3}
}

func (x *InstallRequest) GetCluster() *v1.Reference {
	if x != nil {
		return x.Cluster
	}
	return nil
}

type UninstallRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Cluster *v1.Reference `protobuf:"bytes,1,opt,name=cluster,proto3" json:"cluster,omitempty"`
	Options string        `protobuf:"bytes,2,opt,name=options,proto3" json:"options,omitempty"`
}

func (x *UninstallRequest) Reset() {
	*x = UninstallRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *UninstallRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UninstallRequest) ProtoMessage() {}

func (x *UninstallRequest) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UninstallRequest.ProtoReflect.Descriptor instead.
func (*UninstallRequest) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescGZIP(), []int{4}
}

func (x *UninstallRequest) GetCluster() *v1.Reference {
	if x != nil {
		return x.Cluster
	}
	return nil
}

func (x *UninstallRequest) GetOptions() string {
	if x != nil {
		return x.Options
	}
	return ""
}

type InstallerTemplateResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Template string `protobuf:"bytes,1,opt,name=template,proto3" json:"template,omitempty"`
}

func (x *InstallerTemplateResponse) Reset() {
	*x = InstallerTemplateResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *InstallerTemplateResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InstallerTemplateResponse) ProtoMessage() {}

func (x *InstallerTemplateResponse) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InstallerTemplateResponse.ProtoReflect.Descriptor instead.
func (*InstallerTemplateResponse) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescGZIP(), []int{5}
}

func (x *InstallerTemplateResponse) GetTemplate() string {
	if x != nil {
		return x.Template
	}
	return ""
}

type StatusRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Cluster *v1.Reference `protobuf:"bytes,1,opt,name=cluster,proto3" json:"cluster,omitempty"`
}

func (x *StatusRequest) Reset() {
	*x = StatusRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StatusRequest) ProtoMessage() {}

func (x *StatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StatusRequest.ProtoReflect.Descriptor instead.
func (*StatusRequest) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescGZIP(), []int{6}
}

func (x *StatusRequest) GetCluster() *v1.Reference {
	if x != nil {
		return x.Cluster
	}
	return nil
}

type NodeCapabilityStatus struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Enabled  bool                   `protobuf:"varint,1,opt,name=enabled,proto3" json:"enabled,omitempty"`
	LastSync *timestamppb.Timestamp `protobuf:"bytes,2,opt,name=lastSync,proto3" json:"lastSync,omitempty"`
}

func (x *NodeCapabilityStatus) Reset() {
	*x = NodeCapabilityStatus{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *NodeCapabilityStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NodeCapabilityStatus) ProtoMessage() {}

func (x *NodeCapabilityStatus) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use NodeCapabilityStatus.ProtoReflect.Descriptor instead.
func (*NodeCapabilityStatus) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescGZIP(), []int{7}
}

func (x *NodeCapabilityStatus) GetEnabled() bool {
	if x != nil {
		return x.Enabled
	}
	return false
}

func (x *NodeCapabilityStatus) GetLastSync() *timestamppb.Timestamp {
	if x != nil {
		return x.LastSync
	}
	return nil
}

var File_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto protoreflect.FileDescriptor

var file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDesc = []byte{
	0x0a, 0x3f, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e,
	0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70,
	0x69, 0x73, 0x2f, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x2f, 0x76, 0x31,
	0x2f, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x12, 0x0a, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x1a, 0x1b, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65,
	0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x75, 0x72, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65,
	0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x33, 0x67, 0x69, 0x74,
	0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x65, 0x72, 0x2f,
	0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x73, 0x2f, 0x63, 0x6f,
	0x72, 0x65, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x1a, 0x2a, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6b, 0x72, 0x61,
	0x6c, 0x69, 0x63, 0x6b, 0x79, 0x2f, 0x74, 0x6f, 0x74, 0x65, 0x6d, 0x2f, 0x65, 0x78, 0x74, 0x65,
	0x6e, 0x73, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x64, 0x0a, 0x0b,
	0x53, 0x79, 0x6e, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x29, 0x0a, 0x07, 0x63,
	0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x63,
	0x6f, 0x72, 0x65, 0x2e, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65, 0x52, 0x07, 0x63,
	0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x12, 0x2a, 0x0a, 0x06, 0x66, 0x69, 0x6c, 0x74, 0x65, 0x72,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c,
	0x69, 0x74, 0x79, 0x2e, 0x46, 0x69, 0x6c, 0x74, 0x65, 0x72, 0x52, 0x06, 0x66, 0x69, 0x6c, 0x74,
	0x65, 0x72, 0x22, 0x32, 0x0a, 0x06, 0x46, 0x69, 0x6c, 0x74, 0x65, 0x72, 0x12, 0x28, 0x0a, 0x0f,
	0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x4e, 0x61, 0x6d, 0x65, 0x73, 0x18,
	0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0f, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74,
	0x79, 0x4e, 0x61, 0x6d, 0x65, 0x73, 0x22, 0x36, 0x0a, 0x0c, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x26, 0x0a, 0x0e, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69,
	0x6c, 0x69, 0x74, 0x79, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e,
	0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x4e, 0x61, 0x6d, 0x65, 0x22, 0x3b,
	0x0a, 0x0e, 0x49, 0x6e, 0x73, 0x74, 0x61, 0x6c, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x12, 0x29, 0x0a, 0x07, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x0f, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e,
	0x63, 0x65, 0x52, 0x07, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x22, 0x57, 0x0a, 0x10, 0x55,
	0x6e, 0x69, 0x6e, 0x73, 0x74, 0x61, 0x6c, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12,
	0x29, 0x0a, 0x07, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x0f, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63,
	0x65, 0x52, 0x07, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x12, 0x18, 0x0a, 0x07, 0x6f, 0x70,
	0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6f, 0x70, 0x74,
	0x69, 0x6f, 0x6e, 0x73, 0x22, 0x37, 0x0a, 0x19, 0x49, 0x6e, 0x73, 0x74, 0x61, 0x6c, 0x6c, 0x65,
	0x72, 0x54, 0x65, 0x6d, 0x70, 0x6c, 0x61, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x12, 0x1a, 0x0a, 0x08, 0x74, 0x65, 0x6d, 0x70, 0x6c, 0x61, 0x74, 0x65, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x08, 0x74, 0x65, 0x6d, 0x70, 0x6c, 0x61, 0x74, 0x65, 0x22, 0x3a, 0x0a,
	0x0d, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x29,
	0x0a, 0x07, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x0f, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65,
	0x52, 0x07, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x22, 0x68, 0x0a, 0x14, 0x4e, 0x6f, 0x64,
	0x65, 0x43, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x12, 0x18, 0x0a, 0x07, 0x65, 0x6e, 0x61, 0x62, 0x6c, 0x65, 0x64, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x08, 0x52, 0x07, 0x65, 0x6e, 0x61, 0x62, 0x6c, 0x65, 0x64, 0x12, 0x36, 0x0a, 0x08, 0x6c,
	0x61, 0x73, 0x74, 0x53, 0x79, 0x6e, 0x63, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x08, 0x6c, 0x61, 0x73, 0x74, 0x53,
	0x79, 0x6e, 0x63, 0x32, 0x9a, 0x04, 0x0a, 0x07, 0x42, 0x61, 0x63, 0x6b, 0x65, 0x6e, 0x64, 0x12,
	0x38, 0x0a, 0x04, 0x49, 0x6e, 0x66, 0x6f, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a,
	0x18, 0x2e, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x2e, 0x49, 0x6e, 0x66,
	0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x41, 0x0a, 0x0a, 0x43, 0x61, 0x6e,
	0x49, 0x6e, 0x73, 0x74, 0x61, 0x6c, 0x6c, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a,
	0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x03, 0x88, 0x02, 0x01, 0x12, 0x3d, 0x0a, 0x07,
	0x49, 0x6e, 0x73, 0x74, 0x61, 0x6c, 0x6c, 0x12, 0x1a, 0x2e, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69,
	0x6c, 0x69, 0x74, 0x79, 0x2e, 0x49, 0x6e, 0x73, 0x74, 0x61, 0x6c, 0x6c, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x45, 0x0a, 0x06, 0x53,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x19, 0x2e, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69,
	0x74, 0x79, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x20, 0x2e, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x2e, 0x4e, 0x6f,
	0x64, 0x65, 0x43, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x53, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x12, 0x41, 0x0a, 0x09, 0x55, 0x6e, 0x69, 0x6e, 0x73, 0x74, 0x61, 0x6c, 0x6c, 0x12,
	0x1c, 0x2e, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x2e, 0x55, 0x6e, 0x69,
	0x6e, 0x73, 0x74, 0x61, 0x6c, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x16, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x34, 0x0a, 0x0f, 0x55, 0x6e, 0x69, 0x6e, 0x73, 0x74, 0x61,
	0x6c, 0x6c, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x0f, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e,
	0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65, 0x1a, 0x10, 0x2e, 0x63, 0x6f, 0x72, 0x65,
	0x2e, 0x54, 0x61, 0x73, 0x6b, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x3a, 0x0a, 0x0f, 0x43,
	0x61, 0x6e, 0x63, 0x65, 0x6c, 0x55, 0x6e, 0x69, 0x6e, 0x73, 0x74, 0x61, 0x6c, 0x6c, 0x12, 0x0f,
	0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65, 0x1a,
	0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x57, 0x0a, 0x11, 0x49, 0x6e, 0x73, 0x74, 0x61,
	0x6c, 0x6c, 0x65, 0x72, 0x54, 0x65, 0x6d, 0x70, 0x6c, 0x61, 0x74, 0x65, 0x12, 0x16, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45,
	0x6d, 0x70, 0x74, 0x79, 0x1a, 0x25, 0x2e, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74,
	0x79, 0x2e, 0x49, 0x6e, 0x73, 0x74, 0x61, 0x6c, 0x6c, 0x65, 0x72, 0x54, 0x65, 0x6d, 0x70, 0x6c,
	0x61, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x03, 0x88, 0x02, 0x01,
	0x32, 0x45, 0x0a, 0x04, 0x4e, 0x6f, 0x64, 0x65, 0x12, 0x3d, 0x0a, 0x07, 0x53, 0x79, 0x6e, 0x63,
	0x4e, 0x6f, 0x77, 0x12, 0x12, 0x2e, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79,
	0x2e, 0x46, 0x69, 0x6c, 0x74, 0x65, 0x72, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22,
	0x06, 0x8a, 0xf1, 0x04, 0x02, 0x08, 0x01, 0x32, 0x4d, 0x0a, 0x0b, 0x4e, 0x6f, 0x64, 0x65, 0x4d,
	0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x12, 0x3e, 0x0a, 0x0b, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x53, 0x79, 0x6e, 0x63, 0x12, 0x17, 0x2e, 0x63, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69,
	0x74, 0x79, 0x2e, 0x53, 0x79, 0x6e, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x16,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x42, 0x30, 0x5a, 0x2e, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e,
	0x69, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x73, 0x2f, 0x63, 0x61, 0x70, 0x61, 0x62,
	0x69, 0x6c, 0x69, 0x74, 0x79, 0x2f, 0x76, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescOnce sync.Once
	file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescData = file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDesc
)

func file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescGZIP() []byte {
	file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescOnce.Do(func() {
		file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescData = protoimpl.X.CompressGZIP(file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescData)
	})
	return file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDescData
}

var file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes = make([]protoimpl.MessageInfo, 8)
var file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_goTypes = []interface{}{
	(*SyncRequest)(nil),               // 0: capability.SyncRequest
	(*Filter)(nil),                    // 1: capability.Filter
	(*InfoResponse)(nil),              // 2: capability.InfoResponse
	(*InstallRequest)(nil),            // 3: capability.InstallRequest
	(*UninstallRequest)(nil),          // 4: capability.UninstallRequest
	(*InstallerTemplateResponse)(nil), // 5: capability.InstallerTemplateResponse
	(*StatusRequest)(nil),             // 6: capability.StatusRequest
	(*NodeCapabilityStatus)(nil),      // 7: capability.NodeCapabilityStatus
	(*v1.Reference)(nil),              // 8: core.Reference
	(*timestamppb.Timestamp)(nil),     // 9: google.protobuf.Timestamp
	(*emptypb.Empty)(nil),             // 10: google.protobuf.Empty
	(*v1.TaskStatus)(nil),             // 11: core.TaskStatus
}
var file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_depIdxs = []int32{
	8,  // 0: capability.SyncRequest.cluster:type_name -> core.Reference
	1,  // 1: capability.SyncRequest.filter:type_name -> capability.Filter
	8,  // 2: capability.InstallRequest.cluster:type_name -> core.Reference
	8,  // 3: capability.UninstallRequest.cluster:type_name -> core.Reference
	8,  // 4: capability.StatusRequest.cluster:type_name -> core.Reference
	9,  // 5: capability.NodeCapabilityStatus.lastSync:type_name -> google.protobuf.Timestamp
	10, // 6: capability.Backend.Info:input_type -> google.protobuf.Empty
	10, // 7: capability.Backend.CanInstall:input_type -> google.protobuf.Empty
	3,  // 8: capability.Backend.Install:input_type -> capability.InstallRequest
	6,  // 9: capability.Backend.Status:input_type -> capability.StatusRequest
	4,  // 10: capability.Backend.Uninstall:input_type -> capability.UninstallRequest
	8,  // 11: capability.Backend.UninstallStatus:input_type -> core.Reference
	8,  // 12: capability.Backend.CancelUninstall:input_type -> core.Reference
	10, // 13: capability.Backend.InstallerTemplate:input_type -> google.protobuf.Empty
	1,  // 14: capability.Node.SyncNow:input_type -> capability.Filter
	0,  // 15: capability.NodeManager.RequestSync:input_type -> capability.SyncRequest
	2,  // 16: capability.Backend.Info:output_type -> capability.InfoResponse
	10, // 17: capability.Backend.CanInstall:output_type -> google.protobuf.Empty
	10, // 18: capability.Backend.Install:output_type -> google.protobuf.Empty
	7,  // 19: capability.Backend.Status:output_type -> capability.NodeCapabilityStatus
	10, // 20: capability.Backend.Uninstall:output_type -> google.protobuf.Empty
	11, // 21: capability.Backend.UninstallStatus:output_type -> core.TaskStatus
	10, // 22: capability.Backend.CancelUninstall:output_type -> google.protobuf.Empty
	5,  // 23: capability.Backend.InstallerTemplate:output_type -> capability.InstallerTemplateResponse
	10, // 24: capability.Node.SyncNow:output_type -> google.protobuf.Empty
	10, // 25: capability.NodeManager.RequestSync:output_type -> google.protobuf.Empty
	16, // [16:26] is the sub-list for method output_type
	6,  // [6:16] is the sub-list for method input_type
	6,  // [6:6] is the sub-list for extension type_name
	6,  // [6:6] is the sub-list for extension extendee
	0,  // [0:6] is the sub-list for field type_name
}

func init() { file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_init() }
func file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_init() {
	if File_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SyncRequest); i {
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
		file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Filter); i {
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
		file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*InfoResponse); i {
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
		file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*InstallRequest); i {
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
		file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*UninstallRequest); i {
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
		file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*InstallerTemplateResponse); i {
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
		file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StatusRequest); i {
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
		file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*NodeCapabilityStatus); i {
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
			RawDescriptor: file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   8,
			NumExtensions: 0,
			NumServices:   3,
		},
		GoTypes:           file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_goTypes,
		DependencyIndexes: file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_depIdxs,
		MessageInfos:      file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_msgTypes,
	}.Build()
	File_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto = out.File
	file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_rawDesc = nil
	file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_goTypes = nil
	file_github_com_rancher_opni_pkg_apis_capability_v1_capability_proto_depIdxs = nil
}
