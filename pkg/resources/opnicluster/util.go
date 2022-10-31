package opnicluster

import (
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func (r *Reconciler) serviceLabels(service v1beta2.ServiceKind) map[string]string {
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
		resources.OpniClusterName: r.instanceName,
	}
}

func (r *Reconciler) pretrainedModelLabels(modelName string) map[string]string {
	return map[string]string{
		resources.PretrainedModelLabel: modelName,
	}
}

func (r *Reconciler) serviceImageSpec(service v1beta2.ServiceKind) opnimeta.ImageSpec {
	return opnimeta.ImageResolver{
		Version:             r.spec.Version,
		ImageName:           service.ImageName(),
		DefaultRepo:         "docker.io/rancher",
		DefaultRepoOverride: r.spec.DefaultRepo,
		ImageOverride:       service.GetImageSpec(r.spec),
	}.Resolve()
}

func (r *Reconciler) serviceNodeSelector(service v1beta2.ServiceKind) map[string]string {
	if s := service.GetNodeSelector(r.spec); len(s) > 0 {
		return s
	}
	return r.spec.GlobalNodeSelector
}

func (r *Reconciler) natsNodeSelector() map[string]string {
	if len(r.spec.Nats.NodeSelector) > 0 {
		return r.spec.Nats.NodeSelector
	}
	return r.spec.GlobalNodeSelector
}

func (r *Reconciler) serviceTolerations(service v1beta2.ServiceKind) []corev1.Toleration {
	return append(r.spec.GlobalTolerations, service.GetTolerations(r.spec)...)
}

func (r *Reconciler) natsTolerations() []corev1.Toleration {
	return append(r.spec.GlobalTolerations, r.spec.Nats.Tolerations...)
}

func addCPUInferenceLabel(deployment *appsv1.Deployment) {
	deployment.Labels[resources.OpniInferenceType] = "cpu"
	deployment.Spec.Template.Labels[resources.OpniInferenceType] = "cpu"
	deployment.Spec.Selector.MatchLabels[resources.OpniInferenceType] = "cpu"
}

func ConvertSpec(input aiv1beta1.OpniClusterSpec) v1beta2.OpniClusterSpec {
	retSpec := v1beta2.OpniClusterSpec{}

	services := v1beta2.ServicesSpec{}
	services.Inference = v1beta2.InferenceServiceSpec(input.Services.Inference)
	services.PayloadReceiver = v1beta2.PayloadReceiverServiceSpec(input.Services.PayloadReceiver)
	services.GPUController = v1beta2.GPUControllerServiceSpec(input.Services.GPUController)
	services.Metrics = v1beta2.MetricsServiceSpec(input.Services.Metrics)
	services.Preprocessing = v1beta2.PreprocessingServiceSpec(input.Services.Preprocessing)
	services.Drain = v1beta2.DrainServiceSpec(input.Services.Drain)
	services.OpensearchUpdate = v1beta2.OpensearchUpdateServiceSpec(input.Services.OpensearchUpdate)

	retSpec.Services = services
	retSpec.S3 = v1beta2.S3Spec{
		Internal: (*v1beta2.InternalSpec)(input.S3.Internal),
		External: (*v1beta2.ExternalSpec)(input.S3.External),
	}
	retSpec.NulogHyperparameters = input.NulogHyperparameters
	retSpec.DeployLogCollector = input.DeployLogCollector
	retSpec.GlobalNodeSelector = input.GlobalNodeSelector
	retSpec.GlobalTolerations = input.GlobalTolerations
	retSpec.Nats = v1beta2.NatsSpec{
		Replicas:   lo.ToPtr(int32(0)),
		AuthMethod: v1beta2.NatsAuthNkey,
	}
	retSpec.Opensearch = v1beta2.OpensearchClusterSpec{
		ExternalOpensearch:        input.Opensearch,
		EnableLogIndexManagement:  lo.ToPtr(false),
		EnableIngestPreprocessing: false,
	}
	retSpec.DefaultRepo = input.DefaultRepo
	retSpec.Version = input.Version

	return retSpec
}
