package demo

import (
	"fmt"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/model/filter"
	"github.com/banzaicloud/logging-operator/pkg/sdk/model/output"
	demov1alpha1 "github.com/rancher/opni/apis/demo/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KibanaDashboardPodName           = "deploy-opni-kibana-dasbhboards"
	DrainServiceImage                = "rancher/opni-drain-service:v0.1.2"
	NulogInfServiceControlPlaneImage = "rancher/opni-inference-service:v0.1.2"
	NulogInfServiceImage             = "rancher/opni-inference-service:v0.1.2"
	PayloadReceiverServiceImage      = "rancher/opni-payload-receiver-service:v0.1.2"
	GPUServiceControllerImage        = "rancher/opni-gpu-service-controller:v0.1.2"
	PreprocessingServiceImage        = "rancher/opni-preprocessing-service:v0.1.2"
	KibanaDashboardImage             = "rancher/opni-kibana-dashboard:v0.1.2"
)

func BuildDrainService(spec *demov1alpha1.OpniDemo) *appsv1.Deployment {
	labels := map[string]string{"app": "drain-service"}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "drain-service",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "drain-service",
							Image:           DrainServiceImage,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{
									Name:  "NATS_SERVER_URL",
									Value: fmt.Sprintf("nats://nats_client:%s@nats-client.%s.svc:4222", spec.Spec.NatsPassword, spec.Namespace),
								},
								{
									Name:  "MINIO_SERVER_URL",
									Value: fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", spec.Namespace),
								},
								{
									Name:  "MINIO_ACCESS_KEY",
									Value: spec.Spec.MinioAccessKey,
								},
								{
									Name:  "MINIO_SECRET_KEY",
									Value: spec.Spec.MinioSecretKey,
								},
								{
									Name:  "ES_ENDPOINT",
									Value: fmt.Sprintf("https://opendistro-es-client-service.%s.svc.cluster.local:9200", spec.Namespace),
								},
								{
									Name:  "FAIL_KEYWORDS",
									Value: "fail,error,missing,unable",
								},
								{
									Name:  "ES_USERNAME",
									Value: spec.Spec.ElasticsearchUser,
								},
								{
									Name:  "ES_PASSWORD",
									Value: spec.Spec.ElasticsearchPassword,
								},
							},
						},
					},
				},
			},
		},
	}
}

func BuildNulogInferenceServiceControlPlane(spec *demov1alpha1.OpniDemo) *appsv1.Deployment {
	labels := map[string]string{"app": "nulog-inference-service-control-plane"}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nulog-inference-service-control-plane",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "nulog-inference-service-control-plane",
							Image:           NulogInfServiceControlPlaneImage,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{
									Name:  "NATS_SERVER_URL",
									Value: fmt.Sprintf("nats://nats_client:%s@nats-client.%s.svc:4222", spec.Spec.NatsPassword, spec.Namespace),
								},
								{
									Name:  "MINIO_ENDPOINT",
									Value: fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", spec.Namespace),
								},
								{
									Name:  "MINIO_ACCESS_KEY",
									Value: spec.Spec.MinioAccessKey,
								},
								{
									Name:  "MINIO_SECRET_KEY",
									Value: spec.Spec.MinioSecretKey,
								},
								{
									Name:  "ES_ENDPOINT",
									Value: fmt.Sprintf("https://opendistro-es-client-service.%s.svc.cluster.local:9200", spec.Namespace),
								},
								{
									Name:  "MODEL_THRESHOLD",
									Value: "0.6",
								},
								{
									Name:  "MIN_LOG_TOKENS",
									Value: "4",
								},
								{
									Name:  "IS_CONTROL_PLANE_SERVICE",
									Value: "True",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
									corev1.ResourceCPU:    resource.MustParse(spec.Spec.NulogServiceCPURequest),
								},
							},
						},
					},
				},
			},
		},
	}
}

func BuildNulogInferenceService(spec *demov1alpha1.OpniDemo) *appsv1.Deployment {
	labels := map[string]string{"app": "nulog-inference-service"}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nulog-inference-service",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "nulog-inference-service",
							Image:           NulogInfServiceImage,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{
									Name:  "NATS_SERVER_URL",
									Value: fmt.Sprintf("nats://nats_client:%s@nats-client.%s.svc:4222", spec.Spec.NatsPassword, spec.Namespace),
								},
								{
									Name:  "MINIO_ENDPOINT",
									Value: fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", spec.Namespace),
								},
								{
									Name:  "MINIO_ACCESS_KEY",
									Value: spec.Spec.MinioAccessKey,
								},
								{
									Name:  "MINIO_SECRET_KEY",
									Value: spec.Spec.MinioSecretKey,
								},
								{
									Name:  "ES_ENDPOINT",
									Value: fmt.Sprintf("https://opendistro-es-client-service.%s.svc.cluster.local:9200", spec.Namespace),
								},
								{
									Name:  "MODEL_THRESHOLD",
									Value: "0.5",
								},
								{
									Name:  "MIN_LOG_TOKENS",
									Value: "5",
								},
							},
						},
					},
				},
			},
		},
	}
}

func BuildPayloadReceiverService(spec *demov1alpha1.OpniDemo) (*corev1.Service, *appsv1.Deployment) {
	labels := map[string]string{
		"app": "payload-receiver-service",
	}
	return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "payload-receiver-service",
				Labels: map[string]string{
					"service": "payload-receiver-service",
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: labels,
				Ports: []corev1.ServicePort{
					{
						Port: 80,
					},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "payload-receiver-service",
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            "payload-receiver-service",
								Image:           PayloadReceiverServiceImage,
								ImagePullPolicy: corev1.PullAlways,
								Env: []corev1.EnvVar{
									{
										Name:  "NATS_SERVER_URL",
										Value: fmt.Sprintf("nats://nats_client:%s@nats-client.%s.svc:4222", spec.Spec.NatsPassword, spec.Namespace),
									},
								},
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: 80,
									},
								},
							},
						},
					},
				},
			},
		}
}

func BuildPreprocessingService(spec *demov1alpha1.OpniDemo) *appsv1.Deployment {
	labels := map[string]string{
		"app": "preprocessing-service",
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "preprocessing-service",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "preprocessing-service",
							Image:           PreprocessingServiceImage,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{
									Name:  "NATS_SERVER_URL",
									Value: fmt.Sprintf("nats://nats_client:%s@nats-client.%s.svc:4222", spec.Spec.NatsPassword, spec.Namespace),
								},
								{
									Name:  "ES_ENDPOINT",
									Value: fmt.Sprintf("https://opendistro-es-client-service.%s.svc.cluster.local:9200", spec.Namespace),
								},
								{
									Name:  "ES_USERNAME",
									Value: spec.Spec.ElasticsearchUser,
								},
								{
									Name:  "ES_PASSWORD",
									Value: spec.Spec.ElasticsearchPassword,
								},
							},
						},
					},
				},
			},
		},
	}
}

func BuildGPUService(spec *demov1alpha1.OpniDemo) *appsv1.Deployment {
	labels := map[string]string{"app": "gpu-service"}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gpu-service",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "gpu-service-controller",
							Image:           GPUServiceControllerImage,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{
									Name:  "NATS_SERVER_URL",
									Value: fmt.Sprintf("nats://nats_client:%s@nats-client.%s.svc:4222", spec.Spec.NatsPassword, spec.Namespace),
								},
								{
									Name:  "MINIO_SERVER_URL",
									Value: fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", spec.Namespace),
								},
								{
									Name:  "MINIO_ACCESS_KEY",
									Value: spec.Spec.MinioAccessKey,
								},
								{
									Name:  "MINIO_SECRET_KEY",
									Value: spec.Spec.MinioSecretKey,
								},
								{
									Name:  "ES_ENDPOINT",
									Value: fmt.Sprintf("https://opendistro-es-client-service.%s.svc.cluster.local:9200", spec.Namespace),
								},
								{
									Name:  "ES_USERNAME",
									Value: spec.Spec.ElasticsearchUser,
								},
								{
									Name:  "ES_PASSWORD",
									Value: spec.Spec.ElasticsearchPassword,
								},
								{
									Name:  "NODE_TLS_REJECT_UNAUTHORIZED",
									Value: "0",
								},
							},
						},
						{
							Name:            "gpu-service-worker",
							Image:           NulogInfServiceImage,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{
									Name:  "NATS_SERVER_URL",
									Value: fmt.Sprintf("nats://nats_client:%s@nats-client.%s.svc:4222", spec.Spec.NatsPassword, spec.Namespace),
								},
								{
									Name:  "MINIO_ENDPOINT",
									Value: fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", spec.Namespace),
								},
								{
									Name:  "MINIO_ACCESS_KEY",
									Value: spec.Spec.MinioAccessKey,
								},
								{
									Name:  "MINIO_SECRET_KEY",
									Value: spec.Spec.MinioSecretKey,
								},
								{
									Name:  "ES_ENDPOINT",
									Value: fmt.Sprintf("https://opendistro-es-client-service.%s.svc.cluster.local:9200", spec.Namespace),
								},
								{
									Name:  "MODEL_THRESHOLD",
									Value: "0.5",
								},
								{
									Name:  "MIN_LOG_TOKENS",
									Value: "5",
								},
								{
									Name:  "IS_GPU_SERVICE",
									Value: "True",
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									"nvidia.com/gpu": resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func BuildKibanaDashboardPod(spec *demov1alpha1.OpniDemo) *corev1.Pod {
	labels := map[string]string{"app": "preset-kibana-dashboard"}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   KibanaDashboardPodName,
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "kibana-dashboard",
					Image:           KibanaDashboardImage,
					ImagePullPolicy: corev1.PullAlways,
					Env: []corev1.EnvVar{
						{
							Name:  "ES_ENDPOINT",
							Value: fmt.Sprintf("https://opendistro-es-client-service.%s.svc.cluster.local:9200", spec.Namespace),
						},
						{
							Name:  "ES_USER",
							Value: spec.Spec.ElasticsearchUser,
						},
						{
							Name:  "ES_PASSWORD",
							Value: spec.Spec.ElasticsearchUser,
						},
						{
							Name:  "KB_ENDPOINT",
							Value: fmt.Sprintf("http://opendistro-es-kibana-svc.%s.svc.cluster.local:443", spec.Namespace),
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}

const (
	ClusterOutputName = "aiops-demo-log-output"
	ClusterFlowName   = "aiops-demo-log-flow"
)

func BuildClusterFlow(spec *demov1alpha1.OpniDemo) *loggingv1beta1.ClusterFlow {
	controlNamespace := spec.Namespace
	if !spec.Spec.Components.Opni.RancherLogging.Enabled {
		if ns := spec.Spec.LoggingCRDNamespace; ns != nil {
			controlNamespace = *ns
		}
	}

	return &loggingv1beta1.ClusterFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClusterFlowName,
			Namespace: controlNamespace,
		},
		Spec: loggingv1beta1.ClusterFlowSpec{
			Match: []loggingv1beta1.ClusterMatch{
				{
					ClusterExclude: &loggingv1beta1.ClusterExclude{
						Namespaces: []string{
							spec.Namespace,
						},
					},
				},
				{
					ClusterSelect: &loggingv1beta1.ClusterSelect{},
				},
			},
			Filters: []loggingv1beta1.Filter{
				{
					Dedot: &filter.DedotFilterConfig{
						Separator: "-",
						Nested:    true,
					},
				},
				{
					Grep: &filter.GrepConfig{
						Exclude: []filter.ExcludeSection{
							{
								Key:     "log",
								Pattern: `^\n$`,
							},
						},
					},
				},
				{
					DetectExceptions: &filter.DetectExceptions{
						Languages: []string{
							"java",
							"python",
							"go",
							"ruby",
							"js",
							"csharp",
							"php",
						},
						MultilineFlushInterval: "0.1",
					},
				},
			},
			GlobalOutputRefs: []string{
				ClusterOutputName,
			},
		},
	}
}

func BuildClusterOutput(spec *demov1alpha1.OpniDemo) *loggingv1beta1.ClusterOutput {
	controlNamespace := spec.Namespace
	if !spec.Spec.Components.Opni.RancherLogging.Enabled {
		if ns := spec.Spec.LoggingCRDNamespace; ns != nil {
			controlNamespace = *ns
		}
	}

	return &loggingv1beta1.ClusterOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClusterOutputName,
			Namespace: controlNamespace,
		},
		Spec: loggingv1beta1.ClusterOutputSpec{
			OutputSpec: loggingv1beta1.OutputSpec{
				HTTPOutput: &output.HTTPOutputConfig{
					Endpoint:    fmt.Sprintf("http://payload-receiver-service.%s.svc", spec.Namespace),
					ContentType: "application/json",
					JsonArray:   true,
					Buffer: &output.Buffer{
						Tags:           "[]",
						FlushInterval:  "2s",
						ChunkLimitSize: "1mb",
					},
				},
			},
		},
	}
}
