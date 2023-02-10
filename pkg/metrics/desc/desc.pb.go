// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1-devel
// 	protoc        v1.0.0
// source: github.com/rancher/opni/pkg/metrics/desc/desc.proto

package desc

import (
	_go "github.com/prometheus/client_model/go"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Desc struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	FQName          string           `protobuf:"bytes,1,opt,name=FQName,proto3" json:"FQName,omitempty"`
	Help            string           `protobuf:"bytes,2,opt,name=Help,proto3" json:"Help,omitempty"`
	ConstLabelPairs []*_go.LabelPair `protobuf:"bytes,3,rep,name=ConstLabelPairs,proto3" json:"ConstLabelPairs,omitempty"`
	VariableLabels  []string         `protobuf:"bytes,4,rep,name=VariableLabels,proto3" json:"VariableLabels,omitempty"`
	ID              uint64           `protobuf:"varint,5,opt,name=ID,proto3" json:"ID,omitempty"`
	DimHash         uint64           `protobuf:"varint,6,opt,name=DimHash,proto3" json:"DimHash,omitempty"`
	XPadding1       uint64           `protobuf:"fixed64,7,opt,name=_padding1,json=Padding1,proto3" json:"_padding1,omitempty"`
	XPadding2       uint64           `protobuf:"fixed64,8,opt,name=_padding2,json=Padding2,proto3" json:"_padding2,omitempty"`
}

func (x *Desc) Reset() {
	*x = Desc{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Desc) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Desc) ProtoMessage() {}

func (x *Desc) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Desc.ProtoReflect.Descriptor instead.
func (*Desc) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDescGZIP(), []int{0}
}

func (x *Desc) GetFQName() string {
	if x != nil {
		return x.FQName
	}
	return ""
}

func (x *Desc) GetHelp() string {
	if x != nil {
		return x.Help
	}
	return ""
}

func (x *Desc) GetConstLabelPairs() []*_go.LabelPair {
	if x != nil {
		return x.ConstLabelPairs
	}
	return nil
}

func (x *Desc) GetVariableLabels() []string {
	if x != nil {
		return x.VariableLabels
	}
	return nil
}

func (x *Desc) GetID() uint64 {
	if x != nil {
		return x.ID
	}
	return 0
}

func (x *Desc) GetDimHash() uint64 {
	if x != nil {
		return x.DimHash
	}
	return 0
}

func (x *Desc) GetXPadding1() uint64 {
	if x != nil {
		return x.XPadding1
	}
	return 0
}

func (x *Desc) GetXPadding2() uint64 {
	if x != nil {
		return x.XPadding2
	}
	return 0
}

var File_github_com_rancher_opni_pkg_metrics_desc_desc_proto protoreflect.FileDescriptor

var file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDesc = []byte{
	0x0a, 0x33, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e,
	0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x6d, 0x65,
	0x74, 0x72, 0x69, 0x63, 0x73, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x04, 0x64, 0x65, 0x73, 0x63, 0x1a, 0x45, 0x67, 0x69, 0x74,
	0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x70, 0x72, 0x6f, 0x6d, 0x65, 0x74, 0x68, 0x65,
	0x75, 0x73, 0x2f, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x5f, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x2f,
	0x69, 0x6f, 0x2f, 0x70, 0x72, 0x6f, 0x6d, 0x65, 0x74, 0x68, 0x65, 0x75, 0x73, 0x2f, 0x63, 0x6c,
	0x69, 0x65, 0x6e, 0x74, 0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x73, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x22, 0x89, 0x02, 0x0a, 0x04, 0x44, 0x65, 0x73, 0x63, 0x12, 0x16, 0x0a, 0x06, 0x46,
	0x51, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x46, 0x51, 0x4e,
	0x61, 0x6d, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x48, 0x65, 0x6c, 0x70, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x04, 0x48, 0x65, 0x6c, 0x70, 0x12, 0x49, 0x0a, 0x0f, 0x43, 0x6f, 0x6e, 0x73, 0x74,
	0x4c, 0x61, 0x62, 0x65, 0x6c, 0x50, 0x61, 0x69, 0x72, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x0b,
	0x32, 0x1f, 0x2e, 0x69, 0x6f, 0x2e, 0x70, 0x72, 0x6f, 0x6d, 0x65, 0x74, 0x68, 0x65, 0x75, 0x73,
	0x2e, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x2e, 0x4c, 0x61, 0x62, 0x65, 0x6c, 0x50, 0x61, 0x69,
	0x72, 0x52, 0x0f, 0x43, 0x6f, 0x6e, 0x73, 0x74, 0x4c, 0x61, 0x62, 0x65, 0x6c, 0x50, 0x61, 0x69,
	0x72, 0x73, 0x12, 0x26, 0x0a, 0x0e, 0x56, 0x61, 0x72, 0x69, 0x61, 0x62, 0x6c, 0x65, 0x4c, 0x61,
	0x62, 0x65, 0x6c, 0x73, 0x18, 0x04, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0e, 0x56, 0x61, 0x72, 0x69,
	0x61, 0x62, 0x6c, 0x65, 0x4c, 0x61, 0x62, 0x65, 0x6c, 0x73, 0x12, 0x0e, 0x0a, 0x02, 0x49, 0x44,
	0x18, 0x05, 0x20, 0x01, 0x28, 0x04, 0x52, 0x02, 0x49, 0x44, 0x12, 0x18, 0x0a, 0x07, 0x44, 0x69,
	0x6d, 0x48, 0x61, 0x73, 0x68, 0x18, 0x06, 0x20, 0x01, 0x28, 0x04, 0x52, 0x07, 0x44, 0x69, 0x6d,
	0x48, 0x61, 0x73, 0x68, 0x12, 0x1b, 0x0a, 0x09, 0x5f, 0x70, 0x61, 0x64, 0x64, 0x69, 0x6e, 0x67,
	0x31, 0x18, 0x07, 0x20, 0x01, 0x28, 0x06, 0x52, 0x08, 0x50, 0x61, 0x64, 0x64, 0x69, 0x6e, 0x67,
	0x31, 0x12, 0x1b, 0x0a, 0x09, 0x5f, 0x70, 0x61, 0x64, 0x64, 0x69, 0x6e, 0x67, 0x32, 0x18, 0x08,
	0x20, 0x01, 0x28, 0x06, 0x52, 0x08, 0x50, 0x61, 0x64, 0x64, 0x69, 0x6e, 0x67, 0x32, 0x42, 0x2a,
	0x5a, 0x28, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e,
	0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x6d, 0x65,
	0x74, 0x72, 0x69, 0x63, 0x73, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
}

var (
	file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDescOnce sync.Once
	file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDescData = file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDesc
)

func file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDescGZIP() []byte {
	file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDescOnce.Do(func() {
		file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDescData = protoimpl.X.CompressGZIP(file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDescData)
	})
	return file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDescData
}

var file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_goTypes = []interface{}{
	(*Desc)(nil),          // 0: desc.Desc
	(*_go.LabelPair)(nil), // 1: io.prometheus.client.LabelPair
}
var file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_depIdxs = []int32{
	1, // 0: desc.Desc.ConstLabelPairs:type_name -> io.prometheus.client.LabelPair
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_init() }
func file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_init() {
	if File_github_com_rancher_opni_pkg_metrics_desc_desc_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Desc); i {
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
			RawDescriptor: file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_goTypes,
		DependencyIndexes: file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_depIdxs,
		MessageInfos:      file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_msgTypes,
	}.Build()
	File_github_com_rancher_opni_pkg_metrics_desc_desc_proto = out.File
	file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_rawDesc = nil
	file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_goTypes = nil
	file_github_com_rancher_opni_pkg_metrics_desc_desc_proto_depIdxs = nil
}
