package alertmanager

import (
	"google.golang.org/protobuf/proto"
)

func (in *MultitenantAlertmanagerConfig) DeepCopyInto(out *MultitenantAlertmanagerConfig) {
	proto.Merge(out, in)
}

func (in *MultitenantAlertmanagerConfig) DeepCopy() *MultitenantAlertmanagerConfig {
	return proto.Clone(in).(*MultitenantAlertmanagerConfig)
}
