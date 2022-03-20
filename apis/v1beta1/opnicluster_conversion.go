package v1beta1

import (
	"github.com/rancher/opni/apis/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *OpniCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.OpniCluster)

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Version = src.Spec.Version
	dst.Spec.DefaultRepo = src.Spec.DefaultRepo

	services := v1beta2.ServicesSpec{}
	services.Drain = v1beta2.DrainServiceSpec(src.Spec.Services.Drain)
	services.Inference = v1beta2.InferenceServiceSpec(src.Spec.Services.Inference)
	services.Preprocessing = v1beta2.PreprocessingServiceSpec(src.Spec.Services.Preprocessing)
	services.PayloadReceiver = v1beta2.PayloadReceiverServiceSpec(src.Spec.Services.PayloadReceiver)
	services.GPUController = v1beta2.GPUControllerServiceSpec(src.Spec.Services.GPUController)
	services.Metrics = v1beta2.MetricsServiceSpec(src.Spec.Services.Metrics)
	services.Insights = v1beta2.InsightsServiceSpec(src.Spec.Services.Insights)
	services.UI = v1beta2.UIServiceSpec(src.Spec.Services.UI)
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

	elastic := v1beta2.ElasticSpec{}
	elastic.Version = src.Spec.Elastic.Version
	elastic.DefaultRepo = src.Spec.Elastic.DefaultRepo
	elastic.Image = src.Spec.Elastic.Image
	elastic.KibanaImage = src.Spec.Elastic.KibanaImage
	elastic.Persistence = src.Spec.Elastic.Persistence
	elastic.ConfigSecret = src.Spec.Elastic.ConfigSecret
	elastic.AdminPasswordFrom = src.Spec.Elastic.AdminPasswordFrom

	workloads := v1beta2.ElasticWorkloadSpec{}
	workloads.Master = v1beta2.ElasticWorkloadOptions(src.Spec.Elastic.Workloads.Master)
	workloads.Data = v1beta2.ElasticWorkloadOptions(src.Spec.Elastic.Workloads.Data)
	workloads.Client = v1beta2.ElasticWorkloadOptions(src.Spec.Elastic.Workloads.Client)
	workloads.Kibana = v1beta2.ElasticWorkloadOptions(src.Spec.Elastic.Workloads.Kibana)

	elastic.ExternalOpensearch = nil

	dst.Spec.Elastic = elastic

	return nil
}

//nolint:golint
func (dst *OpniCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.OpniCluster)

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Version = src.Spec.Version
	dst.Spec.DefaultRepo = src.Spec.DefaultRepo

	services := ServicesSpec{}
	services.Drain = DrainServiceSpec(src.Spec.Services.Drain)
	services.Inference = InferenceServiceSpec(src.Spec.Services.Inference)
	services.Preprocessing = PreprocessingServiceSpec(src.Spec.Services.Preprocessing)
	services.PayloadReceiver = PayloadReceiverServiceSpec(src.Spec.Services.PayloadReceiver)
	services.GPUController = GPUControllerServiceSpec(src.Spec.Services.GPUController)
	services.Metrics = MetricsServiceSpec(src.Spec.Services.Metrics)
	services.Insights = InsightsServiceSpec(src.Spec.Services.Insights)
	services.UI = UIServiceSpec(src.Spec.Services.UI)
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
	elastic.Version = src.Spec.Elastic.Version
	elastic.DefaultRepo = src.Spec.Elastic.DefaultRepo
	elastic.Image = src.Spec.Elastic.Image
	elastic.KibanaImage = src.Spec.Elastic.KibanaImage
	elastic.Persistence = src.Spec.Elastic.Persistence
	elastic.ConfigSecret = src.Spec.Elastic.ConfigSecret
	elastic.AdminPasswordFrom = src.Spec.Elastic.AdminPasswordFrom

	workloads := ElasticWorkloadSpec{}
	workloads.Master = ElasticWorkloadOptions(src.Spec.Elastic.Workloads.Master)
	workloads.Data = ElasticWorkloadOptions(src.Spec.Elastic.Workloads.Data)
	workloads.Client = ElasticWorkloadOptions(src.Spec.Elastic.Workloads.Client)
	workloads.Kibana = ElasticWorkloadOptions(src.Spec.Elastic.Workloads.Kibana)

	dst.Spec.Elastic = elastic

	return nil
}
