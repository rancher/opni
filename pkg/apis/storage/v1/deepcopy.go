package v1

import (
	"google.golang.org/protobuf/proto"
)

func (in *StorageSpec) DeepCopyInto(out *StorageSpec) {
	proto.Merge(out, in)
}

func (in *StorageSpec) DeepCopy() *StorageSpec {
	return proto.Clone(in).(*StorageSpec)
}

// func (in *S3StorageSpec) DeepCopyInto(out *S3StorageSpec) {
// 	proto.Merge(out, in)
// }

// func (in *S3StorageSpec) DeepCopy() *S3StorageSpec {
// 	return util.ProtoClone(in)
// }

// func (in *SSEConfig) DeepCopyInto(out *SSEConfig) {
// 	proto.Merge(out, in)
// }

// func (in *SSEConfig) DeepCopy() *SSEConfig {
// 	return util.ProtoClone(in)
// }

// func (in *HTTPConfig) DeepCopyInto(out *HTTPConfig) {
// 	proto.Merge(out, in)
// }

// func (in *HTTPConfig) DeepCopy() *HTTPConfig {
// 	return util.ProtoClone(in)
// }

// func (in *GCSStorageSpec) DeepCopyInto(out *GCSStorageSpec) {
// 	proto.Merge(out, in)
// }

// func (in *GCSStorageSpec) DeepCopy() *GCSStorageSpec {
// 	return util.ProtoClone(in)
// }

// func (in *AzureStorageSpec) DeepCopyInto(out *AzureStorageSpec) {
// 	proto.Merge(out, in)
// }

// func (in *AzureStorageSpec) DeepCopy() *AzureStorageSpec {
// 	return util.ProtoClone(in)
// }

// func (in *SwiftStorageSpec) DeepCopyInto(out *SwiftStorageSpec) {
// 	proto.Merge(out, in)
// }

// func (in *SwiftStorageSpec) DeepCopy() *SwiftStorageSpec {
// 	return util.ProtoClone(in)
// }

// func (in *FilesystemStorageSpec) DeepCopyInto(out *FilesystemStorageSpec) {
// 	proto.Merge(out, in)
// }

// func (in *FilesystemStorageSpec) DeepCopy() *FilesystemStorageSpec {
// 	return util.ProtoClone(in)
// }
