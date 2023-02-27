package resources

const (
	PretrainedModelLabel = "opni.io/pretrained-model"
	ServiceLabel         = "opni.io/service"
	OpniClusterName      = "opni.io/cluster-name"
	AppNameLabel         = "app.kubernetes.io/name"
	PartOfLabel          = "app.kubernetes.io/part-of"
	InstanceLabel        = "app.kubernetes.io/instance"
	HostTopologyKey      = "kubernetes.io/hostname"
	OpniClusterID        = "opni.io/cluster-id"
	OpniBootstrapToken   = "opni.io/bootstrap-token"
	OpniInferenceType    = "opni.io/inference-type"
	OpniConfigHash       = "opni.io/config-hash"
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

func NewGatewayLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": "opni-gateway",
	}
}
