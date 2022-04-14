package core

import "google.golang.org/protobuf/proto"

func (in *BootstrapToken) DeepCopyInto(out *BootstrapToken) {
	proto.Merge(out, in)
}

func (in *Cluster) DeepCopyInto(out *Cluster) {
	proto.Merge(out, in)
}

func (in *Role) DeepCopyInto(out *Role) {
	proto.Merge(out, in)
}

func (in *RoleBinding) DeepCopyInto(out *RoleBinding) {
	proto.Merge(out, in)
}

func (in *BootstrapToken) DeepCopy() *BootstrapToken {
	return proto.Clone(in).(*BootstrapToken)
}

func (in *Cluster) DeepCopy() *Cluster {
	return proto.Clone(in).(*Cluster)
}

func (in *Role) DeepCopy() *Role {
	return proto.Clone(in).(*Role)
}

func (in *RoleBinding) DeepCopy() *RoleBinding {
	return proto.Clone(in).(*RoleBinding)
}
