package opnicluster

import (
	"fmt"
	"strings"
)

const (
	PretrainedModelLabel = "opni.io/pretrained-model"
	ServiceLabel         = "opni.io/service"
	AppNameLabel         = "app.kubernetes.io/name"
	PartOfLabel          = "app.kubernetes.io/part-of"
	InferenceLabel       = "inference"
	DrainLabel           = "drain"
	PreprocessingLabel   = "preprocessing"
	PayloadReceiverLabel = "payload-receiver"
)

func (r *Reconciler) serviceLabels(name string) map[string]string {
	if strings.HasSuffix(name, "-service") {
		panic("service label should not end with -service")
	}
	return map[string]string{
		AppNameLabel: fmt.Sprintf("%s-service", name),
		ServiceLabel: name,
		PartOfLabel:  "opni",
	}
}

func (r *Reconciler) pretrainedModelLabels(name string) map[string]string {
	return map[string]string{
		PretrainedModelLabel: name,
	}
}

func combineLabels(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
