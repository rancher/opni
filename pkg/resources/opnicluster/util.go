package opnicluster

import (
	"fmt"
	"path"

	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"k8s.io/utils/pointer"
)

func (r *Reconciler) serviceLabels(service v1beta1.ServiceKind) map[string]string {
	return map[string]string{
		resources.AppNameLabel: service.ServiceName(),
		resources.ServiceLabel: service.String(),
		resources.PartOfLabel:  "opni",
	}
}

func (r *Reconciler) pretrainedModelLabels(modelName string) map[string]string {
	return map[string]string{
		resources.PretrainedModelLabel: modelName,
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
	return
}
