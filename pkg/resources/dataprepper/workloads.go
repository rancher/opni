package dataprepper

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"

	"github.com/rancher/opni/pkg/resources"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	dataPrepperTemplate = template.Must(template.New("dataprepper").Parse(`log-pipeline:
  workers: 8
  delay: 100
  buffer:
    bounded_blocking:
      buffer_size: 4096
      batch_size: 512
  source:
    http:
      ssl: false
  sink:
  - opensearch:
      hosts: ["{{ .OpensearchEndpoint }}"]
      {{- if .Insecure }}
      insecure: true
      {{- end }}
      username: {{ .Username }}
      password: {{ .Password }}
      index: logs
`))
)

func (r *Reconciler) config() resources.Resource {
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
		return resources.Error(secret, err)
	}

	password, ok := passwordSecret.Data[r.dataPrepper.Spec.PasswordFrom.Key]
	if !ok {
		return resources.Error(secret, errors.New("password secret key does not exist"))
	}

	configData := struct {
		Username           string
		Password           string
		OpensearchEndpoint string
		Insecure           bool
	}{
		Username:           r.dataPrepper.Spec.Username,
		Password:           string(password),
		OpensearchEndpoint: r.dataPrepper.Spec.Opensearch.Endpoint,
		Insecure:           r.dataPrepper.Spec.Opensearch.InsecureDisableSSLVerify,
	}

	var buffer bytes.Buffer

	err = dataPrepperTemplate.Execute(&buffer, configData)
	if err != nil {
		return resources.Error(secret, err)
	}

	secret.Data["pipelines.yaml"] = buffer.Bytes()

	ctrl.SetControllerReference(r.dataPrepper, secret, r.client.Scheme())

	return resources.Present(secret)
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
					Port: 2021,
				},
			},
			Type:     corev1.ServiceTypeClusterIP,
			Selector: r.labels(),
		},
	}

	ctrl.SetControllerReference(r.dataPrepper, service, r.client.Scheme())
	return resources.Present(service)
}

func (r *Reconciler) deployment() resources.Resource {
	imageSpec := opnimeta.ImageResolver{
		Version:             r.dataPrepper.Spec.Version,
		ImageName:           "data-prepper",
		DefaultRepo:         "docker.io/opensearchproject",
		DefaultRepoOverride: r.dataPrepper.Spec.DefaultRepo,
		ImageOverride:       r.dataPrepper.Spec.ImageSpec,
	}.Resolve()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.dataPrepper.Name,
			Namespace: r.dataPrepper.Namespace,
			Labels:    r.labels(),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: r.labels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.labels(),
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
									MountPath: "/usr/share/data-prepper/pipelines.yaml",
									SubPath:   "pipelines.yaml",
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

	ctrl.SetControllerReference(r.dataPrepper, deploy, r.client.Scheme())
	return resources.Present(deploy)
}
