package configv1

import "google.golang.org/protobuf/proto"

func (in *GatewayConfigSpec) DeepCopyInto(out *GatewayConfigSpec) {
	out.Reset()
	proto.Merge(out, in)
}

func (in *GatewayConfigSpec) DeepCopy() *GatewayConfigSpec {
	if in == nil {
		return nil
	}
	out := new(GatewayConfigSpec)
	in.DeepCopyInto(out)
	return out
}
