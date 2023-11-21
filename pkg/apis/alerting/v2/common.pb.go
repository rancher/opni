// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        v1.0.0
// source: github.com/rancher/opni/pkg/apis/alerting/v2/common.proto

package v2

import (
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

type LabelMatcher_Type int32

const (
	LabelMatcher_EQ  LabelMatcher_Type = 0
	LabelMatcher_NEQ LabelMatcher_Type = 1
	LabelMatcher_RE  LabelMatcher_Type = 2
	LabelMatcher_NRE LabelMatcher_Type = 3
)

// Enum value maps for LabelMatcher_Type.
var (
	LabelMatcher_Type_name = map[int32]string{
		0: "EQ",
		1: "NEQ",
		2: "RE",
		3: "NRE",
	}
	LabelMatcher_Type_value = map[string]int32{
		"EQ":  0,
		"NEQ": 1,
		"RE":  2,
		"NRE": 3,
	}
)

func (x LabelMatcher_Type) Enum() *LabelMatcher_Type {
	p := new(LabelMatcher_Type)
	*p = x
	return p
}

func (x LabelMatcher_Type) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (LabelMatcher_Type) Descriptor() protoreflect.EnumDescriptor {
	return file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_enumTypes[0].Descriptor()
}

func (LabelMatcher_Type) Type() protoreflect.EnumType {
	return &file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_enumTypes[0]
}

func (x LabelMatcher_Type) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use LabelMatcher_Type.Descriptor instead.
func (LabelMatcher_Type) EnumDescriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescGZIP(), []int{2, 0}
}

type Label struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name  string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Value string `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
}

func (x *Label) Reset() {
	*x = Label{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Label) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Label) ProtoMessage() {}

func (x *Label) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Label.ProtoReflect.Descriptor instead.
func (*Label) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescGZIP(), []int{0}
}

func (x *Label) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Label) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

type Labels struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Labels []*Label `protobuf:"bytes,1,rep,name=labels,proto3" json:"labels,omitempty"`
}

func (x *Labels) Reset() {
	*x = Labels{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Labels) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Labels) ProtoMessage() {}

func (x *Labels) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Labels.ProtoReflect.Descriptor instead.
func (*Labels) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescGZIP(), []int{1}
}

func (x *Labels) GetLabels() []*Label {
	if x != nil {
		return x.Labels
	}
	return nil
}

// Matcher specifies a rule, which can match or set of labels or not.
type LabelMatcher struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type  LabelMatcher_Type `protobuf:"varint,1,opt,name=type,proto3,enum=alerting.LabelMatcher_Type" json:"type,omitempty"`
	Name  string            `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Value string            `protobuf:"bytes,3,opt,name=value,proto3" json:"value,omitempty"`
}

func (x *LabelMatcher) Reset() {
	*x = LabelMatcher{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LabelMatcher) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LabelMatcher) ProtoMessage() {}

func (x *LabelMatcher) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LabelMatcher.ProtoReflect.Descriptor instead.
func (*LabelMatcher) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescGZIP(), []int{2}
}

func (x *LabelMatcher) GetType() LabelMatcher_Type {
	if x != nil {
		return x.Type
	}
	return LabelMatcher_EQ
}

func (x *LabelMatcher) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *LabelMatcher) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

var File_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto protoreflect.FileDescriptor

var file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDesc = []byte{
	0x0a, 0x39, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e,
	0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70,
	0x69, 0x73, 0x2f, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x69, 0x6e, 0x67, 0x2f, 0x76, 0x32, 0x2f, 0x63,
	0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x61, 0x6c, 0x65,
	0x72, 0x74, 0x69, 0x6e, 0x67, 0x22, 0x31, 0x0a, 0x05, 0x4c, 0x61, 0x62, 0x65, 0x6c, 0x12, 0x12,
	0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61,
	0x6d, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x22, 0x31, 0x0a, 0x06, 0x4c, 0x61, 0x62, 0x65,
	0x6c, 0x73, 0x12, 0x27, 0x0a, 0x06, 0x6c, 0x61, 0x62, 0x65, 0x6c, 0x73, 0x18, 0x01, 0x20, 0x03,
	0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x4c, 0x61,
	0x62, 0x65, 0x6c, 0x52, 0x06, 0x6c, 0x61, 0x62, 0x65, 0x6c, 0x73, 0x22, 0x93, 0x01, 0x0a, 0x0c,
	0x4c, 0x61, 0x62, 0x65, 0x6c, 0x4d, 0x61, 0x74, 0x63, 0x68, 0x65, 0x72, 0x12, 0x2f, 0x0a, 0x04,
	0x74, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x1b, 0x2e, 0x61, 0x6c, 0x65,
	0x72, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x4c, 0x61, 0x62, 0x65, 0x6c, 0x4d, 0x61, 0x74, 0x63, 0x68,
	0x65, 0x72, 0x2e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x12, 0x0a,
	0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d,
	0x65, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x22, 0x28, 0x0a, 0x04, 0x54, 0x79, 0x70, 0x65, 0x12,
	0x06, 0x0a, 0x02, 0x45, 0x51, 0x10, 0x00, 0x12, 0x07, 0x0a, 0x03, 0x4e, 0x45, 0x51, 0x10, 0x01,
	0x12, 0x06, 0x0a, 0x02, 0x52, 0x45, 0x10, 0x02, 0x12, 0x07, 0x0a, 0x03, 0x4e, 0x52, 0x45, 0x10,
	0x03, 0x42, 0x2e, 0x5a, 0x2c, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f,
	0x72, 0x61, 0x6e, 0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x70, 0x6b, 0x67,
	0x2f, 0x61, 0x70, 0x69, 0x73, 0x2f, 0x61, 0x6c, 0x65, 0x72, 0x74, 0x69, 0x6e, 0x67, 0x2f, 0x76,
	0x32, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescOnce sync.Once
	file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescData = file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDesc
)

func file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescGZIP() []byte {
	file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescOnce.Do(func() {
		file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescData = protoimpl.X.CompressGZIP(file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescData)
	})
	return file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDescData
}

var file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_goTypes = []interface{}{
	(LabelMatcher_Type)(0), // 0: alerting.LabelMatcher.Type
	(*Label)(nil),          // 1: alerting.Label
	(*Labels)(nil),         // 2: alerting.Labels
	(*LabelMatcher)(nil),   // 3: alerting.LabelMatcher
}
var file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_depIdxs = []int32{
	1, // 0: alerting.Labels.labels:type_name -> alerting.Label
	0, // 1: alerting.LabelMatcher.type:type_name -> alerting.LabelMatcher.Type
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_init() }
func file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_init() {
	if File_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Label); i {
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
		file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Labels); i {
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
		file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LabelMatcher); i {
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
			RawDescriptor: file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_goTypes,
		DependencyIndexes: file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_depIdxs,
		EnumInfos:         file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_enumTypes,
		MessageInfos:      file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_msgTypes,
	}.Build()
	File_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto = out.File
	file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_rawDesc = nil
	file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_goTypes = nil
	file_github_com_rancher_opni_pkg_apis_alerting_v2_common_proto_depIdxs = nil
}
