package opnicluster

import (
	"fmt"
	"path"

	"github.com/rancher/opni/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

const (
	PretrainedModelLabel = "opni.io/pretrained-model"
	ServiceLabel         = "opni.io/service"
	AppNameLabel         = "app.kubernetes.io/name"
	PartOfLabel          = "app.kubernetes.io/part-of"
)

func (r *Reconciler) serviceLabels(service v1beta1.ServiceKind) map[string]string {
	return map[string]string{
		AppNameLabel: service.ServiceName(),
		ServiceLabel: service.String(),
		PartOfLabel:  "opni",
	}
}

func (r *Reconciler) pretrainedModelLabels(modelName string) map[string]string {
	return map[string]string{
		PretrainedModelLabel: modelName,
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

func (r *Reconciler) serviceImageSpec(service v1beta1.ServiceKind) (result v1beta1.ImageSpec) {
	// If the service provides a custom service image, use that instead.
	if spec := service.GetImageSpec(r.opniCluster); spec != nil {
		if spec.ImagePullPolicy != nil {
			result.ImagePullPolicy = spec.ImagePullPolicy
		}
		if len(spec.ImagePullSecrets) > 0 {
			result.ImagePullSecrets = spec.ImagePullSecrets
		}
		if spec.Image != nil {
			// If image is set, nothing else needs to be done
			result.Image = spec.Image
			return
		}
	}

	// If the service provides a default repo, use that with the default image
	// name and version tag.
	defaultRepo := "docker.io/rancher"
	if r.opniCluster.Spec.DefaultRepo != nil {
		defaultRepo = *r.opniCluster.Spec.DefaultRepo
	}
	version := r.opniCluster.Spec.Version
	if version == "" {
		version = "latest"
	}
	result.Image = pointer.String(fmt.Sprintf("%s:%s",
		path.Join(defaultRepo, service.ImageName()),
		version))
	result.ImagePullPolicy =
		(*corev1.PullPolicy)(pointer.String(string(corev1.PullIfNotPresent)))
	return
}
