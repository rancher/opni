// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1-devel
// 	protoc        v1.0.0
// source: github.com/rancher/opni/plugins/pkg/apis/server/endpoint/alerting.endpoint.proto

package endpoint

import (
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	common "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	_ "google.golang.org/genproto/googleapis/api/annotations"
	_ "google.golang.org/genproto/googleapis/rpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	_ "google.golang.org/protobuf/types/descriptorpb"
	_ "google.golang.org/protobuf/types/known/durationpb"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	_ "google.golang.org/protobuf/types/known/structpb"
	_ "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

var File_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto protoreflect.FileDescriptor

var file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_rawDesc = []byte{
	0x0a, 0x50, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e,
	0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e,
	0x73, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x73, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65,
	0x72, 0x2f, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x2f, 0x61, 0x6c, 0x65, 0x72, 0x74,
	0x69, 0x6e, 0x67, 0x2e, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x12, 0x11, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x65, 0x6e, 0x64,
	0x70, 0x6f, 0x69, 0x6e, 0x74, 0x1a, 0x1e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x73, 0x74, 0x72, 0x75, 0x63, 0x74, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x1a, 0x20, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x1a, 0x15, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f,
	0x68, 0x74, 0x74, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x17, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2f, 0x72, 0x70, 0x63, 0x2f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x1a, 0x33, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61,
	0x6e, 0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x61,
	0x70, 0x69, 0x73, 0x2f, 0x63, 0x6f, 0x72, 0x65, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6f, 0x72, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x45, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63,
	0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f,
	0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x73, 0x2f, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x69, 0x6e, 0x67,
	0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x73, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e,
	0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x32, 0x86, 0x07,
	0x0a, 0x0e, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x73,
	0x12, 0x5b, 0x0a, 0x13, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45,
	0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x12, 0x15, 0x2e, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e,
	0x2e, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x1a, 0x16,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x15, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x0f, 0x3a, 0x01,
	0x2a, 0x22, 0x0a, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x75, 0x72, 0x65, 0x12, 0x51, 0x0a,
	0x10, 0x47, 0x65, 0x74, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e,
	0x74, 0x12, 0x0f, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e,
	0x63, 0x65, 0x1a, 0x15, 0x2e, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x41, 0x6c, 0x65, 0x72,
	0x74, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x22, 0x15, 0x82, 0xd3, 0xe4, 0x93, 0x02,
	0x0f, 0x3a, 0x01, 0x2a, 0x22, 0x0a, 0x2f, 0x6c, 0x69, 0x73, 0x74, 0x2f, 0x7b, 0x69, 0x64, 0x7d,
	0x12, 0x61, 0x0a, 0x12, 0x4c, 0x69, 0x73, 0x74, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45, 0x6e, 0x64,
	0x70, 0x6f, 0x69, 0x6e, 0x74, 0x73, 0x12, 0x21, 0x2e, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e,
	0x4c, 0x69, 0x73, 0x74, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e,
	0x74, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x19, 0x2e, 0x63, 0x6f, 0x6d, 0x6d,
	0x6f, 0x6e, 0x2e, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74,
	0x4c, 0x69, 0x73, 0x74, 0x22, 0x0d, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x07, 0x12, 0x05, 0x2f, 0x6c,
	0x69, 0x73, 0x74, 0x12, 0x68, 0x0a, 0x13, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x41, 0x6c, 0x65,
	0x72, 0x74, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x12, 0x22, 0x2e, 0x63, 0x6f, 0x6d,
	0x6d, 0x6f, 0x6e, 0x2e, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45,
	0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x16,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x15, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x0f, 0x3a, 0x01,
	0x2a, 0x1a, 0x0a, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x75, 0x72, 0x65, 0x12, 0x54, 0x0a,
	0x13, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45, 0x6e, 0x64, 0x70,
	0x6f, 0x69, 0x6e, 0x74, 0x12, 0x0f, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x52, 0x65, 0x66, 0x65,
	0x72, 0x65, 0x6e, 0x63, 0x65, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x14, 0x82,
	0xd3, 0xe4, 0x93, 0x02, 0x0e, 0x22, 0x0c, 0x2f, 0x64, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x2f, 0x7b,
	0x69, 0x64, 0x7d, 0x12, 0x6a, 0x0a, 0x11, 0x54, 0x65, 0x73, 0x74, 0x41, 0x6c, 0x65, 0x72, 0x74,
	0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x12, 0x20, 0x2e, 0x63, 0x6f, 0x6d, 0x6d, 0x6f,
	0x6e, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45, 0x6e, 0x64, 0x70, 0x6f,
	0x69, 0x6e, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x21, 0x2e, 0x63, 0x6f, 0x6d,
	0x6d, 0x6f, 0x6e, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x41, 0x6c, 0x65, 0x72, 0x74, 0x45, 0x6e, 0x64,
	0x70, 0x6f, 0x69, 0x6e, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x10, 0x82,
	0xd3, 0xe4, 0x93, 0x02, 0x0a, 0x3a, 0x01, 0x2a, 0x22, 0x05, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x12,
	0x52, 0x0a, 0x18, 0x4c, 0x69, 0x73, 0x74, 0x52, 0x6f, 0x75, 0x74, 0x69, 0x6e, 0x67, 0x52, 0x65,
	0x6c, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x68, 0x69, 0x70, 0x73, 0x12, 0x16, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d,
	0x70, 0x74, 0x79, 0x1a, 0x1c, 0x2e, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x52, 0x6f, 0x75,
	0x74, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x6c, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x68, 0x69, 0x70,
	0x73, 0x22, 0x00, 0x12, 0x4b, 0x0a, 0x1a, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x43, 0x6f, 0x6e,
	0x64, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x6f, 0x75, 0x74, 0x69, 0x6e, 0x67, 0x4e, 0x6f, 0x64,
	0x65, 0x12, 0x13, 0x2e, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x52, 0x6f, 0x75, 0x74, 0x69,
	0x6e, 0x67, 0x4e, 0x6f, 0x64, 0x65, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x00,
	0x12, 0x4b, 0x0a, 0x1a, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x64, 0x69, 0x74,
	0x69, 0x6f, 0x6e, 0x52, 0x6f, 0x75, 0x74, 0x69, 0x6e, 0x67, 0x4e, 0x6f, 0x64, 0x65, 0x12, 0x13,
	0x2e, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x52, 0x6f, 0x75, 0x74, 0x69, 0x6e, 0x67, 0x4e,
	0x6f, 0x64, 0x65, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x00, 0x12, 0x47, 0x0a,
	0x1a, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x64, 0x69, 0x74, 0x69, 0x6f, 0x6e,
	0x52, 0x6f, 0x75, 0x74, 0x69, 0x6e, 0x67, 0x4e, 0x6f, 0x64, 0x65, 0x12, 0x0f, 0x2e, 0x63, 0x6f,
	0x72, 0x65, 0x2e, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65, 0x1a, 0x16, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45,
	0x6d, 0x70, 0x74, 0x79, 0x22, 0x00, 0x42, 0x3a, 0x5a, 0x38, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e,
	0x69, 0x2f, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x73, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70,
	0x69, 0x73, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69,
	0x6e, 0x74, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_goTypes = []interface{}{
	(*common.AlertEndpoint)(nil),              // 0: common.AlertEndpoint
	(*v1.Reference)(nil),                      // 1: core.Reference
	(*common.ListAlertEndpointsRequest)(nil),  // 2: common.ListAlertEndpointsRequest
	(*common.UpdateAlertEndpointRequest)(nil), // 3: common.UpdateAlertEndpointRequest
	(*common.TestAlertEndpointRequest)(nil),   // 4: common.TestAlertEndpointRequest
	(*emptypb.Empty)(nil),                     // 5: google.protobuf.Empty
	(*common.RoutingNode)(nil),                // 6: common.RoutingNode
	(*common.AlertEndpointList)(nil),          // 7: common.AlertEndpointList
	(*common.TestAlertEndpointResponse)(nil),  // 8: common.TestAlertEndpointResponse
	(*common.RoutingRelationships)(nil),       // 9: common.RoutingRelationships
}
var file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_depIdxs = []int32{
	0,  // 0: alerting.endpoint.AlertEndpoints.CreateAlertEndpoint:input_type -> common.AlertEndpoint
	1,  // 1: alerting.endpoint.AlertEndpoints.GetAlertEndpoint:input_type -> core.Reference
	2,  // 2: alerting.endpoint.AlertEndpoints.ListAlertEndpoints:input_type -> common.ListAlertEndpointsRequest
	3,  // 3: alerting.endpoint.AlertEndpoints.UpdateAlertEndpoint:input_type -> common.UpdateAlertEndpointRequest
	1,  // 4: alerting.endpoint.AlertEndpoints.DeleteAlertEndpoint:input_type -> core.Reference
	4,  // 5: alerting.endpoint.AlertEndpoints.TestAlertEndpoint:input_type -> common.TestAlertEndpointRequest
	5,  // 6: alerting.endpoint.AlertEndpoints.ListRoutingRelationships:input_type -> google.protobuf.Empty
	6,  // 7: alerting.endpoint.AlertEndpoints.CreateConditionRoutingNode:input_type -> common.RoutingNode
	6,  // 8: alerting.endpoint.AlertEndpoints.UpdateConditionRoutingNode:input_type -> common.RoutingNode
	1,  // 9: alerting.endpoint.AlertEndpoints.DeleteConditionRoutingNode:input_type -> core.Reference
	5,  // 10: alerting.endpoint.AlertEndpoints.CreateAlertEndpoint:output_type -> google.protobuf.Empty
	0,  // 11: alerting.endpoint.AlertEndpoints.GetAlertEndpoint:output_type -> common.AlertEndpoint
	7,  // 12: alerting.endpoint.AlertEndpoints.ListAlertEndpoints:output_type -> common.AlertEndpointList
	5,  // 13: alerting.endpoint.AlertEndpoints.UpdateAlertEndpoint:output_type -> google.protobuf.Empty
	5,  // 14: alerting.endpoint.AlertEndpoints.DeleteAlertEndpoint:output_type -> google.protobuf.Empty
	8,  // 15: alerting.endpoint.AlertEndpoints.TestAlertEndpoint:output_type -> common.TestAlertEndpointResponse
	9,  // 16: alerting.endpoint.AlertEndpoints.ListRoutingRelationships:output_type -> common.RoutingRelationships
	5,  // 17: alerting.endpoint.AlertEndpoints.CreateConditionRoutingNode:output_type -> google.protobuf.Empty
	5,  // 18: alerting.endpoint.AlertEndpoints.UpdateConditionRoutingNode:output_type -> google.protobuf.Empty
	5,  // 19: alerting.endpoint.AlertEndpoints.DeleteConditionRoutingNode:output_type -> google.protobuf.Empty
	10, // [10:20] is the sub-list for method output_type
	0,  // [0:10] is the sub-list for method input_type
	0,  // [0:0] is the sub-list for extension type_name
	0,  // [0:0] is the sub-list for extension extendee
	0,  // [0:0] is the sub-list for field type_name
}

func init() {
	file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_init()
}
func file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_init() {
	if File_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   0,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_goTypes,
		DependencyIndexes: file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_depIdxs,
	}.Build()
	File_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto = out.File
	file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_rawDesc = nil
	file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_goTypes = nil
	file_github_com_rancher_opni_plugins_pkg_apis_server_endpoint_alerting_endpoint_proto_depIdxs = nil
}
