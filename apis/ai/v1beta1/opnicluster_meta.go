package v1beta1

import (
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	corev1 "k8s.io/api/core/v1"
)

type ServiceKind int

const (
	InferenceService ServiceKind = iota
	DrainService
	PreprocessingService
	PayloadReceiverService
	GPUControllerService
	MetricsService
	TrainingControllerService
	OpensearchUpdateService
)

type OpensearchRole string

const (
	OpensearchDataRole       OpensearchRole = "data"
	OpensearchClientRole     OpensearchRole = "client"
	OpensearchMasterRole     OpensearchRole = "master"
	OpensearchDashboardsRole OpensearchRole = "kibana"
)

func (s ServiceKind) String() string {
	switch s {
	case InferenceService:
		return "inference"
	case DrainService:
		return "drain"
	case PreprocessingService:
		return "preprocessing"
	case PayloadReceiverService:
		return "payload-receiver"
	case GPUControllerService:
		return "gpu-controller"
	case MetricsService:
		return "metrics"
	case OpensearchUpdateService:
		return "opensearch-update"
	case TrainingControllerService:
		return "training-controller"
	default:
		return ""
	}
}

func (s ServiceKind) ServiceName() string {
	return "opni-svc-" + s.String()
}

func (s ServiceKind) ImageName() string {
	switch s {
	case GPUControllerService:
		return "opni-gpu-service-controller"
	default:
		return "opni-" + s.String() + "-service"
	}
}

func (s ServiceKind) GetImageSpec(opniCluster *OpniCluster) *opnimeta.ImageSpec {
	switch s {
	case InferenceService:
		return &opniCluster.Spec.Services.Inference.ImageSpec
	case DrainService:
		return &opniCluster.Spec.Services.Drain.ImageSpec
	case PreprocessingService:
		return &opniCluster.Spec.Services.Preprocessing.ImageSpec
	case PayloadReceiverService:
		return &opniCluster.Spec.Services.PayloadReceiver.ImageSpec
	case GPUControllerService:
		return &opniCluster.Spec.Services.GPUController.ImageSpec
	case MetricsService:
		return &opniCluster.Spec.Services.Metrics.ImageSpec
	case OpensearchUpdateService:
		return &opniCluster.Spec.Services.OpensearchUpdate.ImageSpec
	case TrainingControllerService:
		return &opniCluster.Spec.Services.TrainingController.ImageSpec
	default:
		return nil
	}
}

func (s ServiceKind) GetNodeSelector(opniCluster *OpniCluster) map[string]string {
	switch s {
	case InferenceService:
		return opniCluster.Spec.Services.Inference.NodeSelector
	case DrainService:
		return opniCluster.Spec.Services.Drain.NodeSelector
	case PreprocessingService:
		return opniCluster.Spec.Services.Preprocessing.NodeSelector
	case PayloadReceiverService:
		return opniCluster.Spec.Services.PayloadReceiver.NodeSelector
	case GPUControllerService:
		return opniCluster.Spec.Services.GPUController.NodeSelector
	case MetricsService:
		return opniCluster.Spec.Services.Metrics.NodeSelector
	case OpensearchUpdateService:
		return opniCluster.Spec.Services.OpensearchUpdate.NodeSelector
	case TrainingControllerService:
		return opniCluster.Spec.Services.TrainingController.NodeSelector
	default:
		return map[string]string{}
	}
}

func (s ServiceKind) GetTolerations(opniCluster *OpniCluster) []corev1.Toleration {
	switch s {
	case InferenceService:
		return opniCluster.Spec.Services.Inference.Tolerations
	case DrainService:
		return opniCluster.Spec.Services.Drain.Tolerations
	case PreprocessingService:
		return opniCluster.Spec.Services.Preprocessing.Tolerations
	case PayloadReceiverService:
		return opniCluster.Spec.Services.PayloadReceiver.Tolerations
	case GPUControllerService:
		return opniCluster.Spec.Services.GPUController.Tolerations
	case MetricsService:
		return opniCluster.Spec.Services.Metrics.Tolerations
	case OpensearchUpdateService:
		return opniCluster.Spec.Services.OpensearchUpdate.Tolerations
	case TrainingControllerService:
		return opniCluster.Spec.Services.TrainingController.Tolerations
	default:
		return []corev1.Toleration{}
	}
}

func (c *OpniCluster) GetState() string {
	return string(c.Status.State)
}

func (c *OpniCluster) GetConditions() []string {
	return c.Status.Conditions
}
