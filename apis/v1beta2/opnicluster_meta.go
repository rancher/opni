package v1beta2

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
	InsightsService
	UIService
	OpensearchFetcher
)

type ElasticRole string

const (
	ElasticDataRole   ElasticRole = "data"
	ElasticClientRole ElasticRole = "client"
	ElasticMasterRole ElasticRole = "master"
	ElasticKibanaRole ElasticRole = "kibana"
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
	case InsightsService:
		return "insights"
	case UIService:
		return "ui"
	case OpensearchFetcher:
		return "opensearch-fetcher"
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
	case UIService:
		return "opni-ui"
	case OpensearchFetcher:
		return "opni-opensearch-fetcher"
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
	case InsightsService:
		return &opniCluster.Spec.Services.Insights.ImageSpec
	case UIService:
		return &opniCluster.Spec.Services.UI.ImageSpec
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
	case InsightsService:
		return opniCluster.Spec.Services.Insights.NodeSelector
	case UIService:
		return opniCluster.Spec.Services.UI.NodeSelector
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
	case UIService:
		return opniCluster.Spec.Services.UI.Tolerations
	default:
		return []corev1.Toleration{}
	}
}

func (e ElasticRole) GetNodeSelector(opniCluster *OpniCluster) map[string]string {
	switch e {
	case ElasticDataRole:
		return opniCluster.Spec.Elastic.Workloads.Data.NodeSelector
	case ElasticMasterRole:
		return opniCluster.Spec.Elastic.Workloads.Master.NodeSelector
	case ElasticClientRole:
		return opniCluster.Spec.Elastic.Workloads.Client.NodeSelector
	case ElasticKibanaRole:
		return opniCluster.Spec.Elastic.Workloads.Kibana.NodeSelector
	default:
		return map[string]string{}
	}
}

func (e ElasticRole) GetTolerations(opniCluster *OpniCluster) []corev1.Toleration {
	switch e {
	case ElasticDataRole:
		return opniCluster.Spec.Elastic.Workloads.Data.Tolerations
	case ElasticMasterRole:
		return opniCluster.Spec.Elastic.Workloads.Master.Tolerations
	case ElasticClientRole:
		return opniCluster.Spec.Elastic.Workloads.Client.Tolerations
	case ElasticKibanaRole:
		return opniCluster.Spec.Elastic.Workloads.Kibana.Tolerations
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
