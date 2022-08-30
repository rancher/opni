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
