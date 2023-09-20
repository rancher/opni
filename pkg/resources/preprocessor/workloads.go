package preprocessor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"

	"github.com/rancher/opni/pkg/resources"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	configKey             = "config.yaml"
	preprocessorVersion   = "latest"
	preprocessorImageRepo = "docker.io/jaehnri"
	preprocessorImage     = "otel-collector"
	otlpGRPCPort          = 4317
)

var (
	templatePreprocessorConfig = template.Must(template.New("preprocessor").Parse(`
receivers:
  otlp:
    protocols:
      grpc: {}
      http: {}
processors:
  resource:
    attributes:
    - key: container_image
      action: insert
      from_attribute: container.image.name
    - key: deployment
      action: insert
      from_attribute: k8s.deployment.name
    - key: deployment
      action: insert
      from_attribute: k8s.deployment.name
    - key: pod_name
      action: insert
      from_attribute: k8s.pod.name
    - key: namespace_name
      action: insert
      from_attribute: k8s.namespace.name
    - key: log
      action: delete
  attributes:
    actions:
    - key: filename
      action: insert
      from_attribute: log.file.path
    - key: log.file.path
      action: delete
    - key: message
      action: insert
      from_attribute: log
    - key: log
      action: delete
  transform:
    log_statements:
    - context: log
      statements:
      - merge_maps(attributes, body, "upsert") where attributes["filename"] == nil
      - set(attributes["COMM"], attributes["_COMM"])
      - delete_matching_keys(attributes, "^_.*")
    - context: resource
      statements:
      - set(attributes["log_type"], "controlplane") where attributes["k8s.pod.labels.tier"] == "control-plane"
      - set(attributes["kubernetes_component"], attributes["k8s.pod.labels.component"]) where attributes["k8s.pod.labels.tier"] == "control-plane"
exporters:
  opensearch:
    endpoints: [ "{{ .Endpoint }}" ]
    index: {{ .WriteIndex }}
    mapping:
      mode: flatten_attributes
      timestamp_field: time
      unix_timestamp: true
    tls:
      ca_file: /etc/otel/chain.crt
      cert_file: /etc/otel/certs/tls.crt
      key_file: /etc/otel/certs/tls.key
service:
  pipelines:
    logs:
      receivers: ["otlp"]
      processors: ["resource", "attributes", "transform"]
      exporters: ["opensearch"]
`))
)

type PreprocessorConfig struct {
	Endpoint   string
	WriteIndex string
}

func (r *Reconciler) configMapName() string {
	return fmt.Sprintf("%s-preprocess-config", r.preprocessor.Name)
}

func (r *Reconciler) caChainSecretName() string {
	return fmt.Sprintf("%s-client-ca-chain", r.preprocessor.Name)
}

func (r *Reconciler) opensearchEndpoint() string {
	lg := log.FromContext(r.ctx)

	cluster := &opsterv1.OpenSearchCluster{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.preprocessor.Spec.OpensearchCluster.Name,
		Namespace: r.preprocessor.Namespace,
	}, cluster)
	if err != nil {
		lg.Error(err, "can't get opensearch details")
		return ""
	}
	return fmt.Sprintf("https://%s:9200", cluster.Spec.General.ServiceName)
}

func (r *Reconciler) preprocessorVolumes() (
	retVolumeMounts []corev1.VolumeMount,
	retVolumes []corev1.Volume,
	retErr error,
) {
	retVolumeMounts = append(retVolumeMounts, corev1.VolumeMount{
		Name:      "preprocessor-config",
		MountPath: fmt.Sprintf("/etc/otel/%s", configKey),
		SubPath:   configKey,
	})
	retVolumes = append(retVolumes, corev1.Volume{
		Name: "preprocessor-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.configMapName(),
				},
			},
		},
	})

	retVolumeMounts = append(retVolumeMounts, corev1.VolumeMount{
		Name:      "preprocessor-chain",
		MountPath: "/etc/otel/chain.crt",
		SubPath:   "chain.crt",
	})
	retVolumes = append(retVolumes, corev1.Volume{
		Name: "preprocessor-chain",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: r.caChainSecretName(),
			},
		},
	})

	clientCertRef, retErr := r.certMgr.GetClientCertRef(resources.InternalIndexingUser)
	if retErr != nil {
		return
	}

	retVolumeMounts = append(retVolumeMounts, corev1.VolumeMount{
		Name:      "certs",
		MountPath: "/etc/otel/certs",
	})
	retVolumes = append(retVolumes, corev1.Volume{
		Name: "certs",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: clientCertRef.Name,
			},
		},
	})

	return
}

func (r *Reconciler) configMap() (resources.Resource, string) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.configMapName(),
			Namespace: r.preprocessor.Namespace,
			Labels: map[string]string{
				resources.PartOfLabel: "opni",
			},
		},
		Data: map[string]string{},
	}

	var buffer bytes.Buffer
	err := templatePreprocessorConfig.Execute(&buffer, PreprocessorConfig{
		Endpoint:   r.opensearchEndpoint(),
		WriteIndex: r.preprocessor.Spec.WriteIndex,
	})
	if err != nil {
		return resources.Error(cm, err), ""
	}

	data := buffer.Bytes()
	hash := sha256.New()
	hash.Write(data)
	configHash := hex.EncodeToString(hash.Sum(nil))

	cm.Data[configKey] = string(data)

	ctrl.SetControllerReference(r.preprocessor, cm, r.client.Scheme())

	return resources.Present(cm), configHash
}

func (r *Reconciler) deployment(configHash string) resources.Resource {
	imageSpec := r.imageSpec()
	volmueMounts, volumes, err := r.preprocessorVolumes()
	if err != nil {
		return resources.Error(nil, err)
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-otel-preprocessor", r.preprocessor.Name),
			Namespace: r.preprocessor.Namespace,
			Labels: map[string]string{
				resources.AppNameLabel:  "otel-preprocessor",
				resources.PartOfLabel:   "opni",
				resources.InstanceLabel: r.preprocessor.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: r.preprocessor.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					resources.AppNameLabel:  "otel-preprocessor",
					resources.PartOfLabel:   "opni",
					resources.InstanceLabel: r.preprocessor.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						resources.AppNameLabel:  "otel-preprocessor",
						resources.PartOfLabel:   "opni",
						resources.InstanceLabel: r.preprocessor.Name,
					},
					Annotations: map[string]string{
						resources.OpniConfigHash: configHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "otel-preprocessor",
							Command: []string{
								"/otelcol-custom",
								fmt.Sprintf("--config=/etc/otel/%s", configKey),
							},
							Image:           *imageSpec.Image,
							ImagePullPolicy: imageSpec.GetImagePullPolicy(),
							VolumeMounts:    volmueMounts,
							Ports: []corev1.ContainerPort{
								{
									Name:          "otlp-grpc",
									ContainerPort: otlpGRPCPort,
								},
							},
						},
					},
					ImagePullSecrets: imageSpec.ImagePullSecrets,
					Volumes:          volumes,
				},
			},
		},
	}
	ctrl.SetControllerReference(r.preprocessor, deploy, r.client.Scheme())

	return resources.Present(deploy)
}

func (r *Reconciler) service() resources.Resource {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PreprocessorServiceName(r.preprocessor.Name),
			Namespace: r.preprocessor.Namespace,
			Labels: map[string]string{
				resources.AppNameLabel:  "otel-preprocessor",
				resources.PartOfLabel:   "opni",
				resources.InstanceLabel: r.preprocessor.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "otlp-grpc",
					Protocol:   corev1.ProtocolTCP,
					Port:       otlpGRPCPort,
					TargetPort: intstr.FromInt(int(otlpGRPCPort)),
				},
			},
			Selector: map[string]string{
				resources.AppNameLabel:  "otel-preprocessor",
				resources.PartOfLabel:   "opni",
				resources.InstanceLabel: r.preprocessor.Name,
			},
		},
	}
	ctrl.SetControllerReference(r.preprocessor, svc, r.client.Scheme())

	return resources.Present(svc)
}

func (r *Reconciler) caChain() resources.Resource {
	caRef, err := r.certMgr.GetHTTPCARef()
	if err != nil {
		return resources.Error(nil, err)
	}
	caSecret := &corev1.Secret{}
	err = r.client.Get(r.ctx, types.NamespacedName{
		Name:      caRef.Name,
		Namespace: r.preprocessor.Namespace,
	}, caSecret)
	if err != nil {
		return resources.Error(nil, err)
	}

	chain := caSecret.Data["ca.crt"]
	chain = append(chain, caSecret.Data[corev1.TLSCertKey]...)

	chainSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.caChainSecretName(),
			Namespace: r.preprocessor.Namespace,
		},
		Data: map[string][]byte{
			"chain.crt": chain,
		},
	}
	ctrl.SetControllerReference(r.preprocessor, chainSecret, r.client.Scheme())
	return resources.Present(chainSecret)
}

func (r *Reconciler) imageSpec() opnimeta.ImageSpec {
	return opnimeta.ImageResolver{
		Version:       preprocessorVersion,
		ImageName:     preprocessorImage,
		DefaultRepo:   preprocessorImageRepo,
		ImageOverride: &r.preprocessor.Spec.ImageSpec,
	}.Resolve()
}

func PreprocessorServiceName(instanceName string) string {
	return fmt.Sprintf("%s-preprocess-svc", instanceName)
}
