package v1beta1

import (
	"github.com/rancher/opni/apis/v1beta2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *OpniCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.OpniCluster)

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Version = src.Spec.Version
	dst.Spec.DefaultRepo = src.Spec.DefaultRepo

	services := v1beta2.ServicesSpec{}
	services.Inference = v1beta2.InferenceServiceSpec(src.Spec.Services.Inference)
	services.PayloadReceiver = v1beta2.PayloadReceiverServiceSpec(src.Spec.Services.PayloadReceiver)
	services.GPUController = v1beta2.GPUControllerServiceSpec(src.Spec.Services.GPUController)
	services.Metrics = v1beta2.MetricsServiceSpec(src.Spec.Services.Metrics)

	services.Preprocessing = v1beta2.PreprocessingServiceSpec{
		ImageSpec:    src.Spec.Services.Preprocessing.ImageSpec,
		Enabled:      src.Spec.Services.Preprocessing.Enabled,
		NodeSelector: src.Spec.Services.Preprocessing.NodeSelector,
		Tolerations:  src.Spec.Services.Preprocessing.Tolerations,
	}
	services.Drain = v1beta2.DrainServiceSpec{
		ImageSpec:    src.Spec.Services.Drain.ImageSpec,
		Enabled:      src.Spec.Services.Drain.Enabled,
		NodeSelector: src.Spec.Services.Drain.NodeSelector,
		Tolerations:  src.Spec.Services.Drain.Tolerations,
	}

	dst.Spec.Services = services

	nats := v1beta2.NatsSpec{}
	nats.AuthMethod = v1beta2.NatsAuthMethod(src.Spec.Nats.AuthMethod)
	nats.Username = src.Spec.Nats.Username
	nats.Replicas = src.Spec.Nats.Replicas
	nats.PasswordFrom = src.Spec.Nats.PasswordFrom
	nats.NodeSelector = src.Spec.Nats.NodeSelector
	nats.Tolerations = src.Spec.Nats.Tolerations
	dst.Spec.Nats = nats

	s3 := v1beta2.S3Spec{}

	if src.Spec.S3.Internal == nil {
		s3.Internal = nil
	} else {
		internal := v1beta2.InternalSpec(*src.Spec.S3.Internal)
		s3.Internal = &internal
	}
	if src.Spec.S3.External == nil {
		s3.External = nil
	} else {
		external := v1beta2.ExternalSpec(*src.Spec.S3.External)
		s3.External = &external
	}
	s3.NulogS3Bucket = src.Spec.S3.NulogS3Bucket
	s3.DrainS3Bucket = src.Spec.S3.DrainS3Bucket
	dst.Spec.S3 = s3

	dst.Spec.NulogHyperparameters = src.Spec.NulogHyperparameters
	dst.Spec.DeployLogCollector = src.Spec.DeployLogCollector
	dst.Spec.GlobalNodeSelector = src.Spec.GlobalNodeSelector
	dst.Spec.GlobalTolerations = src.Spec.GlobalTolerations

	elastic := v1beta2.OpensearchClusterSpec{}
	elastic.Version = src.Spec.Elastic.Version
	elastic.DefaultRepo = src.Spec.Elastic.DefaultRepo
	elastic.Image = src.Spec.Elastic.Image
	elastic.DashboardsImage = src.Spec.Elastic.KibanaImage
	elastic.Persistence = src.Spec.Elastic.Persistence
	elastic.ConfigSecret = src.Spec.Elastic.ConfigSecret
	elastic.AdminPasswordFrom = src.Spec.Elastic.AdminPasswordFrom

	workloads := v1beta2.OpensearchWorkloadSpec{}
	workloads.Master = v1beta2.OpensearchWorkloadOptions(src.Spec.Elastic.Workloads.Master)
	workloads.Data = v1beta2.OpensearchWorkloadOptions(src.Spec.Elastic.Workloads.Data)
	workloads.Client = v1beta2.OpensearchWorkloadOptions(src.Spec.Elastic.Workloads.Client)
	workloads.Dashboards = v1beta2.OpensearchWorkloadOptions(src.Spec.Elastic.Workloads.Kibana)

	elastic.ExternalOpensearch = nil

	dst.Spec.Opensearch = elastic

	return nil
}

//nolint:golint
func (dst *OpniCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.OpniCluster)

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Version = src.Spec.Version
	dst.Spec.DefaultRepo = src.Spec.DefaultRepo

	services := ServicesSpec{}
	services.Inference = InferenceServiceSpec(src.Spec.Services.Inference)
	services.PayloadReceiver = PayloadReceiverServiceSpec(src.Spec.Services.PayloadReceiver)
	services.GPUController = GPUControllerServiceSpec(src.Spec.Services.GPUController)
	services.Metrics = MetricsServiceSpec(src.Spec.Services.Metrics)
	services.Insights = InsightsServiceSpec{
		Enabled: pointer.BoolPtr(false),
	}
	services.UI = UIServiceSpec{
		Enabled: pointer.BoolPtr(false),
	}
	services.Preprocessing = PreprocessingServiceSpec{
		ImageSpec:    src.Spec.Services.Preprocessing.ImageSpec,
		Enabled:      src.Spec.Services.Preprocessing.Enabled,
		NodeSelector: src.Spec.Services.Preprocessing.NodeSelector,
		Tolerations:  src.Spec.Services.Preprocessing.Tolerations,
	}
	services.Drain = DrainServiceSpec{
		ImageSpec:    src.Spec.Services.Drain.ImageSpec,
		Enabled:      src.Spec.Services.Drain.Enabled,
		NodeSelector: src.Spec.Services.Drain.NodeSelector,
		Tolerations:  src.Spec.Services.Drain.Tolerations,
	}

	dst.Spec.Services = services

	nats := NatsSpec{}
	nats.AuthMethod = NatsAuthMethod(src.Spec.Nats.AuthMethod)
	nats.Username = src.Spec.Nats.Username
	nats.Replicas = src.Spec.Nats.Replicas
	nats.PasswordFrom = src.Spec.Nats.PasswordFrom
	nats.NodeSelector = src.Spec.Nats.NodeSelector
	nats.Tolerations = src.Spec.Nats.Tolerations
	dst.Spec.Nats = nats

	s3 := S3Spec{}

	if src.Spec.S3.Internal == nil {
		s3.Internal = nil
	} else {
		internal := InternalSpec(*src.Spec.S3.Internal)
		s3.Internal = &internal
	}
	if src.Spec.S3.External == nil {
		s3.External = nil
	} else {
		external := ExternalSpec(*src.Spec.S3.External)
		s3.External = &external
	}
	s3.NulogS3Bucket = src.Spec.S3.NulogS3Bucket
	s3.DrainS3Bucket = src.Spec.S3.DrainS3Bucket
	dst.Spec.S3 = s3

	dst.Spec.NulogHyperparameters = src.Spec.NulogHyperparameters
	dst.Spec.DeployLogCollector = src.Spec.DeployLogCollector
	dst.Spec.GlobalNodeSelector = src.Spec.GlobalNodeSelector
	dst.Spec.GlobalTolerations = src.Spec.GlobalTolerations

	elastic := ElasticSpec{}
	elastic.Version = src.Spec.Opensearch.Version
	elastic.DefaultRepo = src.Spec.Opensearch.DefaultRepo
	elastic.Image = src.Spec.Opensearch.Image
	elastic.KibanaImage = src.Spec.Opensearch.DashboardsImage
	elastic.Persistence = src.Spec.Opensearch.Persistence
	elastic.ConfigSecret = src.Spec.Opensearch.ConfigSecret
	elastic.AdminPasswordFrom = src.Spec.Opensearch.AdminPasswordFrom

	workloads := ElasticWorkloadSpec{}
	workloads.Master = ElasticWorkloadOptions(src.Spec.Opensearch.Workloads.Master)
	workloads.Data = ElasticWorkloadOptions(src.Spec.Opensearch.Workloads.Data)
	workloads.Client = ElasticWorkloadOptions(src.Spec.Opensearch.Workloads.Client)
	workloads.Kibana = ElasticWorkloadOptions(src.Spec.Opensearch.Workloads.Dashboards)

	dst.Spec.Elastic = elastic

	return nil
}
