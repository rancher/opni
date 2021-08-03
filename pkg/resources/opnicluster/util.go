package opnicluster

import (
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
)

func (r *Reconciler) serviceLabels(service v1beta1.ServiceKind) map[string]string {
	return map[string]string{
		resources.AppNameLabel: service.ServiceName(),
		resources.ServiceLabel: service.String(),
		resources.PartOfLabel:  "opni",
	}
}

func (r *Reconciler) natsLabels() map[string]string {
	return map[string]string{
		resources.AppNameLabel:    "nats",
		resources.PartOfLabel:     "opni",
		resources.OpniClusterName: r.opniCluster.Name,
	}
}

func (r *Reconciler) pretrainedModelLabels(modelName string) map[string]string {
	return map[string]string{
		resources.PretrainedModelLabel: modelName,
	}
}

func (r *Reconciler) serviceImageSpec(service v1beta1.ServiceKind) v1beta1.ImageSpec {
	return v1beta1.ImageResolver{
		Version:             r.opniCluster.Spec.Version,
		ImageName:           service.ImageName(),
		DefaultRepo:         "docker.io/rancher",
		DefaultRepoOverride: r.opniCluster.Spec.DefaultRepo,
		ImageOverride:       service.GetImageSpec(r.opniCluster),
	}.Resolve()
}
