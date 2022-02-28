package resources

import (
	"github.com/rancher/opni/apis/v1beta1"
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

type ElasticLabels map[string]string

func NewElasticLabels() ElasticLabels {
	return map[string]string{
		"app": "opendistro-es",
	}
}

func (l ElasticLabels) WithRole(role v1beta1.ElasticRole) ElasticLabels {
	copied := map[string]string{}
	for k, v := range l {
		copied[k] = v
	}
	copied["role"] = string(role)
	return copied
}

func (l ElasticLabels) Role() v1beta1.ElasticRole {
	role, ok := l["role"]
	if !ok {
		return ""
	}
	return v1beta1.ElasticRole(role)
}
