package dataprepper

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"

	"github.com/rancher/opni/pkg/resources"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	configHashAnnotation = "opni.io/config"
	pipelineFilename     = "pipelines.yaml"
)

var (
	dataPrepperTemplate = template.Must(template.New("dataprepper").Parse(`otel-trace-pipeline:
  workers: 8 
  delay: "100" 
  source:
    otel_trace_source:
      ssl: false # Change this to enable encryption in transit
      authentication:
        unauthenticated:
  buffer:
    bounded_blocking:
      buffer_size: 25600
      batch_size: 400
  sink:
    - pipeline:
        name: "raw-pipeline"
    - pipeline:
        name: "service-map-pipeline"
raw-pipeline:
  workers: 8 
  delay: "3000" 
  source:
    pipeline:
      name: "otel-trace-pipeline"
  buffer:
    bounded_blocking:
      buffer_size: 25600
      batch_size: 3200
  processor:
    - otel_trace_raw:
    - otel_trace_group:
        hosts: ["{{ .OpensearchEndpoint }}"]
        {{- if .Insecure }}
        insecure: true
        {{- end }}
        username: {{ .Username }}
        password: {{ .Password }}
  sink:
    - opensearch:
        hosts: ["{{ .OpensearchEndpoint }}"]
        {{- if .Insecure }}
        insecure: true
        {{- end }}
        username: {{ .Username }}
        password: {{ .Password }}
        index_type: trace-analytics-raw
service-map-pipeline:
  workers: 8
  delay: "100"
  source:
    pipeline:
      name: "otel-trace-pipeline"
  processor:
    - service_map:
        window_duration: 180 
  buffer:
    bounded_blocking:
      buffer_size: 25600
      batch_size: 400
  sink:
    - opensearch:
        hosts: ["{{ .OpensearchEndpoint }}"]
        {{- if .Insecure }}
        insecure: true
        {{- end }}
        username: {{ .Username }}
        password: {{ .Password }}
        index_type: trace-analytics-service-map
`))
)

func (r *Reconciler) config() (resources.Resource, []byte) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", r.dataPrepper.Name),
			Namespace: r.dataPrepper.Namespace,
		},
		Data: map[string][]byte{},
	}

	passwordSecret := &corev1.Secret{}

	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.dataPrepper.Spec.PasswordFrom.Name,
		Namespace: r.dataPrepper.Namespace,
	}, passwordSecret)
	if err != nil {
		return resources.Error(secret, err), []byte{}
	}

	username, ok := passwordSecret.Data["username"]
	if !ok {
		return resources.Error(secret, errors.New("username secret key does not exist")), []byte{}
	}
	password, ok := passwordSecret.Data[r.dataPrepper.Spec.PasswordFrom.Key]
	if !ok {
		return resources.Error(secret, errors.New("password secret key does not exist")), []byte{}
	}

	configData := struct {
		Username           string
		Password           string
		OpensearchEndpoint string
		Insecure           bool
		ClusterID          string
		EnableTracing      bool
	}{
		Username:           string(username),
		Password:           string(password),
		OpensearchEndpoint: r.opensearchEndpoint(),
		//Insecure:           r.dataPrepper.Spec.Opensearch.InsecureDisableSSLVerify || r.forceInsecure,
		ClusterID:     "f6cd589f-d88b-4c6e-94e5-b694c9d3fb34", //TODO: fetch from the right source
		EnableTracing: r.dataPrepper.Spec.EnableTracing,
	}

	var buffer bytes.Buffer

	err = dataPrepperTemplate.Execute(&buffer, configData)
	if err != nil {
		return resources.Error(secret, err), []byte{}
	}

	secret.Data[pipelineFilename] = buffer.Bytes()

	ctrl.SetControllerReference(r.dataPrepper, secret, r.client.Scheme())

	return resources.Present(secret), secret.Data[pipelineFilename]
}

func (r *Reconciler) labels() map[string]string {
	return map[string]string{
		resources.AppNameLabel:  "dataprepper",
		resources.PartOfLabel:   "opni",
		resources.OpniClusterID: r.dataPrepper.Spec.ClusterID,
	}
}

func (r *Reconciler) service() resources.Resource {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.dataPrepper.Name,
			Namespace: r.dataPrepper.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "logs",
					Port: 2021,
				},
			},
			Type:     corev1.ServiceTypeClusterIP,
			Selector: r.labels(),
		},
	}

	if r.dataPrepper.Spec.EnableTracing {
		service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{
			Name: "traces",
			Port: 21890,
		})
	}

	ctrl.SetControllerReference(r.dataPrepper, service, r.client.Scheme())

	return resources.Present(service)
}

func (r *Reconciler) deployment(configData []byte) resources.Resource {
	imageSpec := opnimeta.ImageResolver{
		Version:             r.dataPrepper.Spec.Version,
		ImageName:           "data-prepper",
		DefaultRepo:         "docker.io/opensearchproject",
		DefaultRepoOverride: r.dataPrepper.Spec.DefaultRepo,
		ImageOverride:       r.dataPrepper.Spec.ImageSpec,
	}.Resolve()

	hash := sha1.New()
	hash.Write(configData)
	configHash := hex.EncodeToString(hash.Sum(nil))

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.dataPrepper.Name,
			Namespace: r.dataPrepper.Namespace,
			Labels:    r.labels(),
			Annotations: map[string]string{
				configHashAnnotation: configHash,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: r.labels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.labels(),
					Annotations: map[string]string{
						configHashAnnotation: configHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "data-prepper",
							Image: *imageSpec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 2021,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: fmt.Sprintf("/usr/share/data-prepper/pipelines/%s", pipelineFilename),
									SubPath:   pipelineFilename,
								},
							},
						},
					},
					ImagePullSecrets: imageSpec.ImagePullSecrets,
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-config", r.dataPrepper.Name),
								},
							},
						},
					},
					NodeSelector: r.dataPrepper.Spec.NodeSelector,
					Tolerations:  r.dataPrepper.Spec.Tolerations,
				},
			},
		},
	}

	if r.dataPrepper.Spec.EnableTracing {
		deploy.Spec.Template.Spec.Containers[0].Ports = append(deploy.Spec.Template.Spec.Containers[0].Ports, corev1.ContainerPort{
			ContainerPort: 21890,
		})
	}

	ctrl.SetControllerReference(r.dataPrepper, deploy, r.client.Scheme())

	return resources.Present(deploy)
}

func (r *Reconciler) opensearchEndpoint() string {
	lg := log.FromContext(r.ctx)

	cluster := &opsterv1.OpenSearchCluster{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.dataPrepper.Spec.OpensearchCluster.Name,
		Namespace: r.dataPrepper.Namespace,
	}, cluster)
	if err != nil {
		lg.Error(err, "can't get opensearch details")
		return ""
	}
	return fmt.Sprintf("https://%s:9200", cluster.Spec.General.ServiceName)
}
