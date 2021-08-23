package resources

const (
	PretrainedModelLabel = "opni.io/pretrained-model"
	ServiceLabel         = "opni.io/service"
	OpniClusterName      = "opni.io/cluster-name"
	AppNameLabel         = "app.kubernetes.io/name"
	PartOfLabel          = "app.kubernetes.io/part-of"
	HostTopologyKey      = "kubernetes.io/hostname"
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
type ElasticRole string

const (
	ElasticDataRole   ElasticRole = "data"
	ElasticClientRole ElasticRole = "client"
	ElasticMasterRole ElasticRole = "master"
	ElasticKibanaRole ElasticRole = "kibana"
)

func NewElasticLabels() ElasticLabels {
	return map[string]string{
		"app": "opendistro-es",
	}
}

func (l ElasticLabels) WithRole(role ElasticRole) ElasticLabels {
	copied := map[string]string{}
	for k, v := range l {
		copied[k] = v
	}
	copied["role"] = string(role)
	return copied
}

func (l ElasticLabels) Role() ElasticRole {
	if role, ok := l["role"]; !ok {
		return ""
	} else {
		return ElasticRole(role)
	}
}
