// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.30.0-devel
// 	protoc        v1.0.0
// source: github.com/rancher/opni/plugins/aiops/pkg/apis/metricai/metricai.proto

package metricai

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	_ "google.golang.org/protobuf/types/known/structpb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type MetricAIJobStatus struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ClusterId      string   `protobuf:"bytes,1,opt,name=clusterId,proto3" json:"clusterId,omitempty"`
	JobCreateTime  string   `protobuf:"bytes,2,opt,name=jobCreateTime,proto3" json:"jobCreateTime,omitempty"`
	JobId          string   `protobuf:"bytes,3,opt,name=jobId,proto3" json:"jobId,omitempty"`
	Namespaces     []string `protobuf:"bytes,4,rep,name=namespaces,proto3" json:"namespaces,omitempty"`
	JobDescription string   `protobuf:"bytes,5,opt,name=jobDescription,proto3" json:"jobDescription,omitempty"`
}

func (x *MetricAIJobStatus) Reset() {
	*x = MetricAIJobStatus{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MetricAIJobStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MetricAIJobStatus) ProtoMessage() {}

func (x *MetricAIJobStatus) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MetricAIJobStatus.ProtoReflect.Descriptor instead.
func (*MetricAIJobStatus) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescGZIP(), []int{0}
}

func (x *MetricAIJobStatus) GetClusterId() string {
	if x != nil {
		return x.ClusterId
	}
	return ""
}

func (x *MetricAIJobStatus) GetJobCreateTime() string {
	if x != nil {
		return x.JobCreateTime
	}
	return ""
}

func (x *MetricAIJobStatus) GetJobId() string {
	if x != nil {
		return x.JobId
	}
	return ""
}

func (x *MetricAIJobStatus) GetNamespaces() []string {
	if x != nil {
		return x.Namespaces
	}
	return nil
}

func (x *MetricAIJobStatus) GetJobDescription() string {
	if x != nil {
		return x.JobDescription
	}
	return ""
}

type MetricAIJobRunResult struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	JobId               string `protobuf:"bytes,1,opt,name=jobId,proto3" json:"jobId,omitempty"`
	JobRunId            string `protobuf:"bytes,2,opt,name=jobRunId,proto3" json:"jobRunId,omitempty"`
	JobRunResult        string `protobuf:"bytes,3,opt,name=jobRunResult,proto3" json:"jobRunResult,omitempty"`
	JobRunCreateTime    string `protobuf:"bytes,4,opt,name=jobRunCreateTime,proto3" json:"jobRunCreateTime,omitempty"`
	JobRunBaseTime      string `protobuf:"bytes,5,opt,name=jobRunBaseTime,proto3" json:"jobRunBaseTime,omitempty"` // the actual timestamp to run metric anomaly detection. For now it's the same as CreeateTime
	Status              string `protobuf:"bytes,6,opt,name=status,proto3" json:"status,omitempty"`
	JobRunResultDetails string `protobuf:"bytes,7,opt,name=jobRunResultDetails,proto3" json:"jobRunResultDetails,omitempty"`
}

func (x *MetricAIJobRunResult) Reset() {
	*x = MetricAIJobRunResult{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MetricAIJobRunResult) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MetricAIJobRunResult) ProtoMessage() {}

func (x *MetricAIJobRunResult) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MetricAIJobRunResult.ProtoReflect.Descriptor instead.
func (*MetricAIJobRunResult) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescGZIP(), []int{1}
}

func (x *MetricAIJobRunResult) GetJobId() string {
	if x != nil {
		return x.JobId
	}
	return ""
}

func (x *MetricAIJobRunResult) GetJobRunId() string {
	if x != nil {
		return x.JobRunId
	}
	return ""
}

func (x *MetricAIJobRunResult) GetJobRunResult() string {
	if x != nil {
		return x.JobRunResult
	}
	return ""
}

func (x *MetricAIJobRunResult) GetJobRunCreateTime() string {
	if x != nil {
		return x.JobRunCreateTime
	}
	return ""
}

func (x *MetricAIJobRunResult) GetJobRunBaseTime() string {
	if x != nil {
		return x.JobRunBaseTime
	}
	return ""
}

func (x *MetricAIJobRunResult) GetStatus() string {
	if x != nil {
		return x.Status
	}
	return ""
}

func (x *MetricAIJobRunResult) GetJobRunResultDetails() string {
	if x != nil {
		return x.JobRunResultDetails
	}
	return ""
}

type MetricAIIdList struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Items []string `protobuf:"bytes,1,rep,name=items,proto3" json:"items,omitempty"`
}

func (x *MetricAIIdList) Reset() {
	*x = MetricAIIdList{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MetricAIIdList) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MetricAIIdList) ProtoMessage() {}

func (x *MetricAIIdList) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MetricAIIdList.ProtoReflect.Descriptor instead.
func (*MetricAIIdList) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescGZIP(), []int{2}
}

func (x *MetricAIIdList) GetItems() []string {
	if x != nil {
		return x.Items
	}
	return nil
}

type MetricAICreateJobRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ClusterId      string   `protobuf:"bytes,1,opt,name=clusterId,proto3" json:"clusterId,omitempty"`
	Namespaces     []string `protobuf:"bytes,2,rep,name=namespaces,proto3" json:"namespaces,omitempty"`
	JobId          string   `protobuf:"bytes,3,opt,name=jobId,proto3" json:"jobId,omitempty"`
	JobDescription string   `protobuf:"bytes,4,opt,name=jobDescription,proto3" json:"jobDescription,omitempty"`
}

func (x *MetricAICreateJobRequest) Reset() {
	*x = MetricAICreateJobRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MetricAICreateJobRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MetricAICreateJobRequest) ProtoMessage() {}

func (x *MetricAICreateJobRequest) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MetricAICreateJobRequest.ProtoReflect.Descriptor instead.
func (*MetricAICreateJobRequest) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescGZIP(), []int{3}
}

func (x *MetricAICreateJobRequest) GetClusterId() string {
	if x != nil {
		return x.ClusterId
	}
	return ""
}

func (x *MetricAICreateJobRequest) GetNamespaces() []string {
	if x != nil {
		return x.Namespaces
	}
	return nil
}

func (x *MetricAICreateJobRequest) GetJobId() string {
	if x != nil {
		return x.JobId
	}
	return ""
}

func (x *MetricAICreateJobRequest) GetJobDescription() string {
	if x != nil {
		return x.JobDescription
	}
	return ""
}

type MetricAIId struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
}

func (x *MetricAIId) Reset() {
	*x = MetricAIId{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MetricAIId) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MetricAIId) ProtoMessage() {}

func (x *MetricAIId) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MetricAIId.ProtoReflect.Descriptor instead.
func (*MetricAIId) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescGZIP(), []int{4}
}

func (x *MetricAIId) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

type MetricAIAPIResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Status        string `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	SubmittedTime string `protobuf:"bytes,2,opt,name=submittedTime,proto3" json:"submittedTime,omitempty"`
	Description   string `protobuf:"bytes,3,opt,name=description,proto3" json:"description,omitempty"`
}

func (x *MetricAIAPIResponse) Reset() {
	*x = MetricAIAPIResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MetricAIAPIResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MetricAIAPIResponse) ProtoMessage() {}

func (x *MetricAIAPIResponse) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MetricAIAPIResponse.ProtoReflect.Descriptor instead.
func (*MetricAIAPIResponse) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescGZIP(), []int{5}
}

func (x *MetricAIAPIResponse) GetStatus() string {
	if x != nil {
		return x.Status
	}
	return ""
}

func (x *MetricAIAPIResponse) GetSubmittedTime() string {
	if x != nil {
		return x.SubmittedTime
	}
	return ""
}

func (x *MetricAIAPIResponse) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

type MetricAIRunJobResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Status        string `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	SubmittedTime string `protobuf:"bytes,2,opt,name=submittedTime,proto3" json:"submittedTime,omitempty"`
	Description   string `protobuf:"bytes,3,opt,name=description,proto3" json:"description,omitempty"`
	JobRunId      string `protobuf:"bytes,4,opt,name=jobRunId,proto3" json:"jobRunId,omitempty"`
}

func (x *MetricAIRunJobResponse) Reset() {
	*x = MetricAIRunJobResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MetricAIRunJobResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MetricAIRunJobResponse) ProtoMessage() {}

func (x *MetricAIRunJobResponse) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MetricAIRunJobResponse.ProtoReflect.Descriptor instead.
func (*MetricAIRunJobResponse) Descriptor() ([]byte, []int) {
	return file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescGZIP(), []int{6}
}

func (x *MetricAIRunJobResponse) GetStatus() string {
	if x != nil {
		return x.Status
	}
	return ""
}

func (x *MetricAIRunJobResponse) GetSubmittedTime() string {
	if x != nil {
		return x.SubmittedTime
	}
	return ""
}

func (x *MetricAIRunJobResponse) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *MetricAIRunJobResponse) GetJobRunId() string {
	if x != nil {
		return x.JobRunId
	}
	return ""
}

var File_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto protoreflect.FileDescriptor

var file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDesc = []byte{
	0x0a, 0x46, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e,
	0x63, 0x68, 0x65, 0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e,
	0x73, 0x2f, 0x61, 0x69, 0x6f, 0x70, 0x73, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x73,
	0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63,
	0x61, 0x69, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63,
	0x61, 0x69, 0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a,
	0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2f, 0x73, 0x74, 0x72, 0x75, 0x63, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x15, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x68, 0x74, 0x74, 0x70, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69,
	0x2f, 0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x22, 0xb5, 0x01, 0x0a, 0x11, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x4a,
	0x6f, 0x62, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x1c, 0x0a, 0x09, 0x63, 0x6c, 0x75, 0x73,
	0x74, 0x65, 0x72, 0x49, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x63, 0x6c, 0x75,
	0x73, 0x74, 0x65, 0x72, 0x49, 0x64, 0x12, 0x24, 0x0a, 0x0d, 0x6a, 0x6f, 0x62, 0x43, 0x72, 0x65,
	0x61, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x6a,
	0x6f, 0x62, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x14, 0x0a, 0x05,
	0x6a, 0x6f, 0x62, 0x49, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x6a, 0x6f, 0x62,
	0x49, 0x64, 0x12, 0x1e, 0x0a, 0x0a, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x73,
	0x18, 0x04, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0a, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63,
	0x65, 0x73, 0x12, 0x26, 0x0a, 0x0e, 0x6a, 0x6f, 0x62, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70,
	0x74, 0x69, 0x6f, 0x6e, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x6a, 0x6f, 0x62, 0x44,
	0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x8a, 0x02, 0x0a, 0x14, 0x4d,
	0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x4a, 0x6f, 0x62, 0x52, 0x75, 0x6e, 0x52, 0x65, 0x73,
	0x75, 0x6c, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x6a, 0x6f, 0x62, 0x49, 0x64, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x05, 0x6a, 0x6f, 0x62, 0x49, 0x64, 0x12, 0x1a, 0x0a, 0x08, 0x6a, 0x6f, 0x62,
	0x52, 0x75, 0x6e, 0x49, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x6a, 0x6f, 0x62,
	0x52, 0x75, 0x6e, 0x49, 0x64, 0x12, 0x22, 0x0a, 0x0c, 0x6a, 0x6f, 0x62, 0x52, 0x75, 0x6e, 0x52,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x6a, 0x6f, 0x62,
	0x52, 0x75, 0x6e, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x2a, 0x0a, 0x10, 0x6a, 0x6f, 0x62,
	0x52, 0x75, 0x6e, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x18, 0x04, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x10, 0x6a, 0x6f, 0x62, 0x52, 0x75, 0x6e, 0x43, 0x72, 0x65, 0x61, 0x74,
	0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x26, 0x0a, 0x0e, 0x6a, 0x6f, 0x62, 0x52, 0x75, 0x6e, 0x42,
	0x61, 0x73, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x6a,
	0x6f, 0x62, 0x52, 0x75, 0x6e, 0x42, 0x61, 0x73, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x16, 0x0a,
	0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x73,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x30, 0x0a, 0x13, 0x6a, 0x6f, 0x62, 0x52, 0x75, 0x6e, 0x52,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x44, 0x65, 0x74, 0x61, 0x69, 0x6c, 0x73, 0x18, 0x07, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x13, 0x6a, 0x6f, 0x62, 0x52, 0x75, 0x6e, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74,
	0x44, 0x65, 0x74, 0x61, 0x69, 0x6c, 0x73, 0x22, 0x26, 0x0a, 0x0e, 0x4d, 0x65, 0x74, 0x72, 0x69,
	0x63, 0x41, 0x49, 0x49, 0x64, 0x4c, 0x69, 0x73, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x69, 0x74, 0x65,
	0x6d, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x05, 0x69, 0x74, 0x65, 0x6d, 0x73, 0x22,
	0x96, 0x01, 0x0a, 0x18, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x43, 0x72, 0x65, 0x61,
	0x74, 0x65, 0x4a, 0x6f, 0x62, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x1c, 0x0a, 0x09,
	0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x49, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x09, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x49, 0x64, 0x12, 0x1e, 0x0a, 0x0a, 0x6e, 0x61,
	0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0a,
	0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x73, 0x12, 0x14, 0x0a, 0x05, 0x6a, 0x6f,
	0x62, 0x49, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x6a, 0x6f, 0x62, 0x49, 0x64,
	0x12, 0x26, 0x0a, 0x0e, 0x6a, 0x6f, 0x62, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69,
	0x6f, 0x6e, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x6a, 0x6f, 0x62, 0x44, 0x65, 0x73,
	0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x1c, 0x0a, 0x0a, 0x4d, 0x65, 0x74, 0x72,
	0x69, 0x63, 0x41, 0x49, 0x49, 0x64, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x22, 0x75, 0x0a, 0x13, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63,
	0x41, 0x49, 0x41, 0x50, 0x49, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x16, 0x0a,
	0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x73,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x24, 0x0a, 0x0d, 0x73, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x74,
	0x65, 0x64, 0x54, 0x69, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x73, 0x75,
	0x62, 0x6d, 0x69, 0x74, 0x74, 0x65, 0x64, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x20, 0x0a, 0x0b, 0x64,
	0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x94, 0x01,
	0x0a, 0x16, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x52, 0x75, 0x6e, 0x4a, 0x6f, 0x62,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x12, 0x24, 0x0a, 0x0d, 0x73, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x74, 0x65, 0x64, 0x54, 0x69, 0x6d,
	0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x73, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x74,
	0x65, 0x64, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x20, 0x0a, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69,
	0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x64, 0x65, 0x73,
	0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x1a, 0x0a, 0x08, 0x6a, 0x6f, 0x62, 0x52,
	0x75, 0x6e, 0x49, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x6a, 0x6f, 0x62, 0x52,
	0x75, 0x6e, 0x49, 0x64, 0x32, 0xa4, 0x09, 0x0a, 0x08, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41,
	0x49, 0x12, 0x7c, 0x0a, 0x16, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x47, 0x72, 0x61, 0x66, 0x61,
	0x6e, 0x61, 0x44, 0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x12, 0x14, 0x2e, 0x6d, 0x65,
	0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x49,
	0x64, 0x1a, 0x1d, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74,
	0x72, 0x69, 0x63, 0x41, 0x49, 0x41, 0x50, 0x49, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0x2d, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x27, 0x22, 0x25, 0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69,
	0x63, 0x61, 0x69, 0x2f, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x67, 0x72, 0x61, 0x66, 0x61, 0x6e,
	0x61, 0x64, 0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x12,
	0x7c, 0x0a, 0x16, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x47, 0x72, 0x61, 0x66, 0x61, 0x6e, 0x61,
	0x44, 0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x12, 0x14, 0x2e, 0x6d, 0x65, 0x74, 0x72,
	0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x49, 0x64, 0x1a,
	0x1d, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69,
	0x63, 0x41, 0x49, 0x41, 0x50, 0x49, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x2d,
	0x82, 0xd3, 0xe4, 0x93, 0x02, 0x27, 0x2a, 0x25, 0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61,
	0x69, 0x2f, 0x64, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x67, 0x72, 0x61, 0x66, 0x61, 0x6e, 0x61, 0x64,
	0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x12, 0x67, 0x0a,
	0x0e, 0x4c, 0x69, 0x73, 0x74, 0x4e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x73, 0x12,
	0x14, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69,
	0x63, 0x41, 0x49, 0x49, 0x64, 0x1a, 0x18, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69,
	0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x49, 0x64, 0x4c, 0x69, 0x73, 0x74, 0x22,
	0x25, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x1f, 0x12, 0x1d, 0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63,
	0x61, 0x69, 0x2f, 0x6c, 0x69, 0x73, 0x74, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65,
	0x73, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x12, 0x58, 0x0a, 0x08, 0x4c, 0x69, 0x73, 0x74, 0x4a, 0x6f,
	0x62, 0x73, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x18, 0x2e, 0x6d, 0x65, 0x74,
	0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x49, 0x64,
	0x4c, 0x69, 0x73, 0x74, 0x22, 0x1a, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x14, 0x12, 0x12, 0x2f, 0x6d,
	0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2f, 0x6c, 0x69, 0x73, 0x74, 0x6a, 0x6f, 0x62, 0x73,
	0x12, 0x61, 0x0a, 0x0b, 0x4c, 0x69, 0x73, 0x74, 0x4a, 0x6f, 0x62, 0x52, 0x75, 0x6e, 0x73, 0x12,
	0x14, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69,
	0x63, 0x41, 0x49, 0x49, 0x64, 0x1a, 0x18, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69,
	0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x49, 0x64, 0x4c, 0x69, 0x73, 0x74, 0x22,
	0x22, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x1c, 0x12, 0x1a, 0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63,
	0x61, 0x69, 0x2f, 0x6c, 0x69, 0x73, 0x74, 0x6a, 0x6f, 0x62, 0x72, 0x75, 0x6e, 0x73, 0x2f, 0x7b,
	0x69, 0x64, 0x7d, 0x12, 0x7a, 0x0a, 0x09, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x4a, 0x6f, 0x62,
	0x12, 0x22, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72,
	0x69, 0x63, 0x41, 0x49, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x4a, 0x6f, 0x62, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x1a, 0x1d, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e,
	0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x41, 0x50, 0x49, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x22, 0x2a, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x24, 0x3a, 0x01, 0x2a, 0x22, 0x1f,
	0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2f, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65,
	0x6a, 0x6f, 0x62, 0x2f, 0x7b, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x49, 0x64, 0x7d, 0x12,
	0x5f, 0x0a, 0x06, 0x52, 0x75, 0x6e, 0x4a, 0x6f, 0x62, 0x12, 0x14, 0x2e, 0x6d, 0x65, 0x74, 0x72,
	0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x49, 0x64, 0x1a,
	0x20, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69,
	0x63, 0x41, 0x49, 0x52, 0x75, 0x6e, 0x4a, 0x6f, 0x62, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x22, 0x1d, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x17, 0x22, 0x15, 0x2f, 0x6d, 0x65, 0x74, 0x72,
	0x69, 0x63, 0x61, 0x69, 0x2f, 0x72, 0x75, 0x6e, 0x6a, 0x6f, 0x62, 0x2f, 0x7b, 0x69, 0x64, 0x7d,
	0x12, 0x62, 0x0a, 0x09, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x4a, 0x6f, 0x62, 0x12, 0x14, 0x2e,
	0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41,
	0x49, 0x49, 0x64, 0x1a, 0x1d, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d,
	0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x41, 0x50, 0x49, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x20, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x1a, 0x2a, 0x18, 0x2f, 0x6d, 0x65, 0x74,
	0x72, 0x69, 0x63, 0x61, 0x69, 0x2f, 0x64, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x6a, 0x6f, 0x62, 0x2f,
	0x7b, 0x69, 0x64, 0x7d, 0x12, 0x68, 0x0a, 0x0c, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x4a, 0x6f,
	0x62, 0x52, 0x75, 0x6e, 0x12, 0x14, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e,
	0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x49, 0x64, 0x1a, 0x1d, 0x2e, 0x6d, 0x65, 0x74,
	0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x41, 0x50,
	0x49, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x23, 0x82, 0xd3, 0xe4, 0x93, 0x02,
	0x1d, 0x2a, 0x1b, 0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2f, 0x64, 0x65, 0x6c,
	0x65, 0x74, 0x65, 0x6a, 0x6f, 0x62, 0x72, 0x75, 0x6e, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x12, 0x6f,
	0x0a, 0x0f, 0x47, 0x65, 0x74, 0x4a, 0x6f, 0x62, 0x52, 0x75, 0x6e, 0x52, 0x65, 0x73, 0x75, 0x6c,
	0x74, 0x12, 0x14, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74,
	0x72, 0x69, 0x63, 0x41, 0x49, 0x49, 0x64, 0x1a, 0x1e, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63,
	0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x4a, 0x6f, 0x62, 0x52, 0x75,
	0x6e, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x22, 0x26, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x20, 0x12,
	0x1e, 0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2f, 0x67, 0x65, 0x74, 0x6a, 0x6f,
	0x62, 0x72, 0x75, 0x6e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x12,
	0x5a, 0x0a, 0x06, 0x47, 0x65, 0x74, 0x4a, 0x6f, 0x62, 0x12, 0x14, 0x2e, 0x6d, 0x65, 0x74, 0x72,
	0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x41, 0x49, 0x49, 0x64, 0x1a,
	0x1b, 0x2e, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69,
	0x63, 0x41, 0x49, 0x4a, 0x6f, 0x62, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x22, 0x1d, 0x82, 0xd3,
	0xe4, 0x93, 0x02, 0x17, 0x12, 0x15, 0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x2f,
	0x67, 0x65, 0x74, 0x6a, 0x6f, 0x62, 0x2f, 0x7b, 0x69, 0x64, 0x7d, 0x42, 0x39, 0x5a, 0x37, 0x67,
	0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x65,
	0x72, 0x2f, 0x6f, 0x70, 0x6e, 0x69, 0x2f, 0x70, 0x6c, 0x75, 0x67, 0x69, 0x6e, 0x73, 0x2f, 0x61,
	0x69, 0x6f, 0x70, 0x73, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x73, 0x2f, 0x6d, 0x65,
	0x74, 0x72, 0x69, 0x63, 0x61, 0x69, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescOnce sync.Once
	file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescData = file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDesc
)

func file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescGZIP() []byte {
	file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescOnce.Do(func() {
		file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescData = protoimpl.X.CompressGZIP(file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescData)
	})
	return file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDescData
}

var file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes = make([]protoimpl.MessageInfo, 7)
var file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_goTypes = []interface{}{
	(*MetricAIJobStatus)(nil),        // 0: metricai.MetricAIJobStatus
	(*MetricAIJobRunResult)(nil),     // 1: metricai.MetricAIJobRunResult
	(*MetricAIIdList)(nil),           // 2: metricai.MetricAIIdList
	(*MetricAICreateJobRequest)(nil), // 3: metricai.MetricAICreateJobRequest
	(*MetricAIId)(nil),               // 4: metricai.MetricAIId
	(*MetricAIAPIResponse)(nil),      // 5: metricai.MetricAIAPIResponse
	(*MetricAIRunJobResponse)(nil),   // 6: metricai.MetricAIRunJobResponse
	(*emptypb.Empty)(nil),            // 7: google.protobuf.Empty
}
var file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_depIdxs = []int32{
	4,  // 0: metricai.MetricAI.CreateGrafanaDashboard:input_type -> metricai.MetricAIId
	4,  // 1: metricai.MetricAI.DeleteGrafanaDashboard:input_type -> metricai.MetricAIId
	4,  // 2: metricai.MetricAI.ListNamespaces:input_type -> metricai.MetricAIId
	7,  // 3: metricai.MetricAI.ListJobs:input_type -> google.protobuf.Empty
	4,  // 4: metricai.MetricAI.ListJobRuns:input_type -> metricai.MetricAIId
	3,  // 5: metricai.MetricAI.CreateJob:input_type -> metricai.MetricAICreateJobRequest
	4,  // 6: metricai.MetricAI.RunJob:input_type -> metricai.MetricAIId
	4,  // 7: metricai.MetricAI.DeleteJob:input_type -> metricai.MetricAIId
	4,  // 8: metricai.MetricAI.DeleteJobRun:input_type -> metricai.MetricAIId
	4,  // 9: metricai.MetricAI.GetJobRunResult:input_type -> metricai.MetricAIId
	4,  // 10: metricai.MetricAI.GetJob:input_type -> metricai.MetricAIId
	5,  // 11: metricai.MetricAI.CreateGrafanaDashboard:output_type -> metricai.MetricAIAPIResponse
	5,  // 12: metricai.MetricAI.DeleteGrafanaDashboard:output_type -> metricai.MetricAIAPIResponse
	2,  // 13: metricai.MetricAI.ListNamespaces:output_type -> metricai.MetricAIIdList
	2,  // 14: metricai.MetricAI.ListJobs:output_type -> metricai.MetricAIIdList
	2,  // 15: metricai.MetricAI.ListJobRuns:output_type -> metricai.MetricAIIdList
	5,  // 16: metricai.MetricAI.CreateJob:output_type -> metricai.MetricAIAPIResponse
	6,  // 17: metricai.MetricAI.RunJob:output_type -> metricai.MetricAIRunJobResponse
	5,  // 18: metricai.MetricAI.DeleteJob:output_type -> metricai.MetricAIAPIResponse
	5,  // 19: metricai.MetricAI.DeleteJobRun:output_type -> metricai.MetricAIAPIResponse
	1,  // 20: metricai.MetricAI.GetJobRunResult:output_type -> metricai.MetricAIJobRunResult
	0,  // 21: metricai.MetricAI.GetJob:output_type -> metricai.MetricAIJobStatus
	11, // [11:22] is the sub-list for method output_type
	0,  // [0:11] is the sub-list for method input_type
	0,  // [0:0] is the sub-list for extension type_name
	0,  // [0:0] is the sub-list for extension extendee
	0,  // [0:0] is the sub-list for field type_name
}

func init() { file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_init() }
func file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_init() {
	if File_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MetricAIJobStatus); i {
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
		file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MetricAIJobRunResult); i {
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
		file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MetricAIIdList); i {
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
		file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MetricAICreateJobRequest); i {
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
		file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MetricAIId); i {
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
		file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MetricAIAPIResponse); i {
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
		file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MetricAIRunJobResponse); i {
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
			RawDescriptor: file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   7,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_goTypes,
		DependencyIndexes: file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_depIdxs,
		MessageInfos:      file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_msgTypes,
	}.Build()
	File_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto = out.File
	file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_rawDesc = nil
	file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_goTypes = nil
	file_github_com_rancher_opni_plugins_aiops_pkg_apis_metricai_metricai_proto_depIdxs = nil
}
