package resources

import (
	"github.com/rancher/opni/apis/v1beta2"
)

const (
	PretrainedModelLabel = "opni.io/pretrained-model"
	ServiceLabel         = "opni.io/service"
	OpniClusterName      = "opni.io/cluster-name"
	AppNameLabel         = "app.kubernetes.io/name"
	PartOfLabel          = "app.kubernetes.io/part-of"
	HostTopologyKey      = "kubernetes.io/hostname"
	OpniClusterID        = "opni.io/cluster-id"
	OpniBootstrapToken   = "opni.io/bootstrap-token"
	OpniInferenceType    = "opni.io/inference-type"
)

func CombineLabels(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

type OpensearchLabels map[string]string

func NewOpensearchLabels() OpensearchLabels {
	return map[string]string{
		"app": "opensearch",
	}
}

func (l OpensearchLabels) WithRole(role v1beta2.OpensearchRole) OpensearchLabels {
	copied := map[string]string{}
	for k, v := range l {
		copied[k] = v
	}
	copied["role"] = string(role)
	return copied
}

func (l OpensearchLabels) Role() v1beta2.OpensearchRole {
	role, ok := l["role"]
	if !ok {
		return ""
	}
	return v1beta2.OpensearchRole(role)
}
