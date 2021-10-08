package opnicluster

import (
	"fmt"

	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	EventExporterImage = "ghcr.io/opsgenie/kubernetes-event-exporter:v0.10"
)

func (r *Reconciler) eventExporter() ([]resources.Resource, error) {
	labels := map[string]string{
		"app": "event-exporter",
	}

	serviceAcct := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-event-exporter",
			Namespace: r.opniCluster.Namespace,
		},
	}

	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opni-event-exporter:" + r.opniCluster.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "opni-event-viewer",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAcct.Name,
				Namespace: r.opniCluster.Namespace,
			},
		},
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-event-exporter-config",
			Namespace: r.opniCluster.Namespace,
		},
		Data: map[string]string{
			"config.yaml": fmt.Sprintf(`
logLevel: debug
logFormat: json
route:
  routes:
  - match:
    - receiver: opni
receivers:
- name: opni
  webhook:
    endpoint: http://%s.%s.svc
    headers:
      User-Agent: kube-event-exporter 1.0`, v1beta1.PayloadReceiverService.ServiceName(), r.opniCluster.Namespace),
		},
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-event-exporter",
			Namespace: r.opniCluster.Namespace,
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
					ServiceAccountName: serviceAcct.Name,
					Containers: []corev1.Container{
						{
							Name:  "event-exporter",
							Image: EventExporterImage,
							Args: []string{
								"-conf=/data/config.yaml",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: configMap.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ctrl.SetControllerReference(r.opniCluster, configMap, r.client.Scheme())
	ctrl.SetControllerReference(r.opniCluster, deployment, r.client.Scheme())

	return []resources.Resource{
		resources.Present(serviceAcct),
		resources.Present(roleBinding),
		resources.Present(configMap),
		resources.Present(deployment),
	}, nil
}
