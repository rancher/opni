package v1beta2

import (
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	corev1 "k8s.io/api/core/v1"
)

type ServiceKind int

const (
	InferenceService ServiceKind = iota
	PretrainedDrainService
	WorkloadDrainService
	PreprocessingService
	PayloadReceiverService
	GPUControllerService
	MetricsService
	OpensearchUpdateService
	TrainingControllerService
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
	case PretrainedDrainService:
		return "pretrained-drain"
	case WorkloadDrainService:
		return "workload-drain"
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

func (s ServiceKind) GetImageSpec(spec OpniClusterSpec) *opnimeta.ImageSpec {
	switch s {
	case InferenceService:
		return &spec.Services.Inference.ImageSpec
	case PretrainedDrainService:
		return &spec.Services.Drain.ImageSpec
	case WorkloadDrainService:
		return &spec.Services.Drain.ImageSpec
	case PreprocessingService:
		return &spec.Services.Preprocessing.ImageSpec
	case PayloadReceiverService:
		return &spec.Services.PayloadReceiver.ImageSpec
	case GPUControllerService:
		return &spec.Services.GPUController.ImageSpec
	case MetricsService:
		return &spec.Services.Metrics.ImageSpec
	case OpensearchUpdateService:
		return &spec.Services.OpensearchUpdate.ImageSpec
	case TrainingControllerService:
		return &spec.Services.TrainingController.ImageSpec
	default:
		return nil
	}
}

func (s ServiceKind) GetNodeSelector(spec OpniClusterSpec) map[string]string {
	switch s {
	case InferenceService:
		return spec.Services.Inference.NodeSelector
	case PretrainedDrainService:
		return spec.Services.Drain.NodeSelector
	case WorkloadDrainService:
		return spec.Services.Drain.NodeSelector
	case PreprocessingService:
		return spec.Services.Preprocessing.NodeSelector
	case PayloadReceiverService:
		return spec.Services.PayloadReceiver.NodeSelector
	case GPUControllerService:
		return spec.Services.GPUController.NodeSelector
	case MetricsService:
		return spec.Services.Metrics.NodeSelector
	case OpensearchUpdateService:
		return spec.Services.OpensearchUpdate.NodeSelector
	case TrainingControllerService:
		return spec.Services.TrainingController.NodeSelector
	default:
		return map[string]string{}
	}
}

func (s ServiceKind) GetTolerations(spec OpniClusterSpec) []corev1.Toleration {
	switch s {
	case InferenceService:
		return spec.Services.Inference.Tolerations
	case PretrainedDrainService:
		return spec.Services.Drain.Tolerations
	case WorkloadDrainService:
		return spec.Services.Drain.Tolerations
	case PreprocessingService:
		return spec.Services.Preprocessing.Tolerations
	case PayloadReceiverService:
		return spec.Services.PayloadReceiver.Tolerations
	case GPUControllerService:
		return spec.Services.GPUController.Tolerations
	case MetricsService:
		return spec.Services.Metrics.Tolerations
	case OpensearchUpdateService:
		return spec.Services.OpensearchUpdate.Tolerations
	case TrainingControllerService:
		return spec.Services.TrainingController.Tolerations
	default:
		return []corev1.Toleration{}
	}
}

func (e OpensearchRole) GetNodeSelector(opniCluster *OpniCluster) map[string]string {
	switch e {
	case OpensearchDataRole:
		return opniCluster.Spec.Opensearch.Workloads.Data.NodeSelector
	case OpensearchMasterRole:
		return opniCluster.Spec.Opensearch.Workloads.Master.NodeSelector
	case OpensearchClientRole:
		return opniCluster.Spec.Opensearch.Workloads.Client.NodeSelector
	case OpensearchDashboardsRole:
		return opniCluster.Spec.Opensearch.Workloads.Dashboards.NodeSelector
	default:
		return map[string]string{}
	}
}

func (e OpensearchRole) GetTolerations(opniCluster *OpniCluster) []corev1.Toleration {
	switch e {
	case OpensearchDataRole:
		return opniCluster.Spec.Opensearch.Workloads.Data.Tolerations
	case OpensearchMasterRole:
		return opniCluster.Spec.Opensearch.Workloads.Master.Tolerations
	case OpensearchClientRole:
		return opniCluster.Spec.Opensearch.Workloads.Client.Tolerations
	case OpensearchDashboardsRole:
		return opniCluster.Spec.Opensearch.Workloads.Dashboards.Tolerations
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
