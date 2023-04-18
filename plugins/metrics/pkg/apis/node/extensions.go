package node

import (
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func IsDefaultConfig(trailer metadata.MD) bool {
	if len(trailer["is-default-config"]) > 0 {
		return trailer["is-default-config"][0] == "true"
	}
	return false
}

func DefaultConfigMetadata() metadata.MD {
	return metadata.Pairs("is-default-config", "true")
}

func (in *OTELSpec) DeepCopyInto(out *OTELSpec) {
	proto.Merge(out, in)
}
