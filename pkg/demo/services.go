package demo

import (
	"bytes"
	"fmt"
	"html/template"

	demov1alpha1 "github.com/rancher/opni/apis/demo/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
)

const (
	KibanaDashboardPodName           = "deploy-opni-kibana-dasbhboards"
	DrainServiceImage                = "rancher/opni-drain-service:v0.1.3"
	NulogInfServiceControlPlaneImage = "rancher/opni-inference-service:v0.1.3"
	NulogInfServiceImage             = "rancher/opni-inference-service:v0.1.3"
	PayloadReceiverServiceImage      = "rancher/opni-payload-receiver-service:v0.1.3"
	GPUServiceControllerImage        = "rancher/opni-gpu-service-controller:v0.1.3"
	PreprocessingServiceImage        = "rancher/opni-preprocessing-service:v0.1.3"
	KibanaDashboardImage             = "rancher/opni-kibana-dashboard:v0.1.3"
)

var clusterOutputManifest = template.Must(template.New("clusteroutput").Parse(`
apiVersion: logging.banzaicloud.io/v1beta1
kind: ClusterOutput
metadata:
  name: aiops-demo-log-output
spec:
  http:
    buffer:
      chunk_limit_size: 1mb
      flush_interval: 2s
      tags: '[]'
      timekey: ""
    content_type: application/json
    endpoint: http://payload-receiver-service.{{ . }}.svc
    json_array: true
`))

var clusterFlowManifest = template.Must(template.New("clusterflow").Parse(`
apiVersion: logging.banzaicloud.io/v1beta1
kind: ClusterFlow
metadata:
  name: aiops-demo-log-flow
spec:
  filters:
  - dedot:
      de_dot_nested: true
      de_dot_separator: '-'
  - grep:
      exclude:
      - key: log
        pattern: ^\n$
  - detectExceptions:
      languages:
      - java
      - python
      - go
      - ruby
      - js
      - csharp
      - php
      multiline_flush_interval: "0.1"
  globalOutputRefs:
  - aiops-demo-log-output
  match:
  - exclude:
      namespaces:
      - {{ . }}
  - select: {}
`))

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
									Value: fmt.Sprintf("nats://nats-client.%s.svc:4222", spec.Namespace),
								},
								{
									Name:  "NATS_USERNAME",
									Value: "nats_client",
								},
								{
									Name:  "NATS_PASSWORD",
									Value: spec.Spec.NatsPassword,
								},
								{
									Name:  "S3_ENDPOINT",
									Value: fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", spec.Namespace),
								},
								{
									Name:  "S3_ACCESS_KEY",
									Value: spec.Spec.MinioAccessKey,
								},
								{
									Name:  "S3_SECRET_KEY",
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
									Value: fmt.Sprintf("nats://nats-client.%s.svc:4222", spec.Namespace),
								},
								{
									Name:  "NATS_USERNAME",
									Value: "nats_client",
								},
								{
									Name:  "NATS_PASSWORD",
									Value: spec.Spec.NatsPassword,
								},
								{
									Name:  "S3_ENDPOINT",
									Value: fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", spec.Namespace),
								},
								{
									Name:  "S3_ACCESS_KEY",
									Value: spec.Spec.MinioAccessKey,
								},
								{
									Name:  "S3_SECRET_KEY",
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
									Value: fmt.Sprintf("nats://nats-client.%s.svc:4222", spec.Namespace),
								},
								{
									Name:  "NATS_USERNAME",
									Value: "nats_client",
								},
								{
									Name:  "NATS_PASSWORD",
									Value: spec.Spec.NatsPassword,
								},
								{
									Name:  "S3_ENDPOINT",
									Value: fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", spec.Namespace),
								},
								{
									Name:  "S3_ACCESS_KEY",
									Value: spec.Spec.MinioAccessKey,
								},
								{
									Name:  "S3_SECRET_KEY",
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
										Value: fmt.Sprintf("nats://nats-client.%s.svc:4222", spec.Namespace),
									},
									{
										Name:  "NATS_USERNAME",
										Value: "nats_client",
									},
									{
										Name:  "NATS_PASSWORD",
										Value: spec.Spec.NatsPassword,
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
									Value: fmt.Sprintf("nats://nats-client.%s.svc:4222", spec.Namespace),
								},
								{
									Name:  "NATS_USERNAME",
									Value: "nats_client",
								},
								{
									Name:  "NATS_PASSWORD",
									Value: spec.Spec.NatsPassword,
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
									Value: fmt.Sprintf("nats://nats-client.%s.svc:4222", spec.Namespace),
								},
								{
									Name:  "NATS_USERNAME",
									Value: "nats_client",
								},
								{
									Name:  "NATS_PASSWORD",
									Value: spec.Spec.NatsPassword,
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
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/opni-data",
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
									Value: fmt.Sprintf("nats://nats-client.%s.svc:4222", spec.Namespace),
								},
								{
									Name:  "NATS_USERNAME",
									Value: "nats_client",
								},
								{
									Name:  "NATS_PASSWORD",
									Value: spec.Spec.NatsPassword,
								},
								{
									Name:  "S3_ENDPOINT",
									Value: fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", spec.Namespace),
								},
								{
									Name:  "S3_ACCESS_KEY",
									Value: spec.Spec.MinioAccessKey,
								},
								{
									Name:  "S3_SECRET_KEY",
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
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/opni-data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
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

func BuildClusterFlow(spec *demov1alpha1.OpniDemo) *unstructured.Unstructured {
	var buffer bytes.Buffer
	clusterFlowManifest.Execute(&buffer, spec.Namespace)

	obj := &unstructured.Unstructured{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	dec.Decode(buffer.Bytes(), nil, obj)

	controlNamespace := spec.Namespace
	if !spec.Spec.Components.Opni.RancherLogging.Enabled {
		if ns := spec.Spec.LoggingCRDNamespace; ns != nil {
			controlNamespace = *ns
		}
	}

	obj.SetNamespace(controlNamespace)
	return obj
}

func BuildClusterOutput(spec *demov1alpha1.OpniDemo) *unstructured.Unstructured {
	var buffer bytes.Buffer
	clusterOutputManifest.Execute(&buffer, spec.Namespace)

	obj := &unstructured.Unstructured{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	dec.Decode(buffer.Bytes(), nil, obj)

	controlNamespace := spec.Namespace
	if !spec.Spec.Components.Opni.RancherLogging.Enabled {
		if ns := spec.Spec.LoggingCRDNamespace; ns != nil {
			controlNamespace = *ns
		}
	}

	obj.SetNamespace(controlNamespace)
	return obj
}
