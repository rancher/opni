package opnicluster

import (
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
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

func (r *Reconciler) serviceNodeSelector(service v1beta1.ServiceKind) map[string]string {
	if s := service.GetNodeSelector(r.opniCluster); len(s) > 0 {
		return s
	}
	return r.opniCluster.Spec.GlobalNodeSelector
}

func (r *Reconciler) natsNodeSelector() map[string]string {
	if len(r.opniCluster.Spec.Nats.NodeSelector) > 0 {
		return r.opniCluster.Spec.Nats.NodeSelector
	}
	return r.opniCluster.Spec.GlobalNodeSelector
}

func (r *Reconciler) serviceTolerations(service v1beta1.ServiceKind) []corev1.Toleration {
	return append(r.opniCluster.Spec.GlobalTolerations, service.GetTolerations(r.opniCluster)...)
}

func (r *Reconciler) natsTolerations() []corev1.Toleration {
	return append(r.opniCluster.Spec.GlobalTolerations, r.opniCluster.Spec.Nats.Tolerations...)
}
