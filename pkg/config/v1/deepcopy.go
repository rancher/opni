package v1

import "google.golang.org/protobuf/proto"

func (in *GatewayConfigSpec) DeepCopyInto(out *GatewayConfigSpec) {
	out.Reset()
	proto.Merge(out, in)
}
