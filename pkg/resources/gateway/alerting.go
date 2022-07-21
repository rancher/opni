package gateway

import (
	v1beta2 "github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) alerting() []resources.Resource {
	if r.gw.Spec.Alerting == nil {
		// set some sensible defaults
		r.gw.Spec.Alerting = &v1beta2.AlertingSpec{
			Port:        9093,
			Storage:     "500Mi",
			ServiceType: "ClusterIP",
		}
	}

	publicLabels := map[string]string{} // TODO define a set of meaningful labels for this service
	labelWithAlert := func(label map[string]string) map[string]string {
		label["app"] = "opni-alerting"
		return label
	}
	publicLabels = labelWithAlert(publicLabels)

	deploy := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting-internal",
			Namespace: r.gw.Namespace,
			Labels:    publicLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: util.Pointer(r.numAlertingReplicas()),
			Selector: &metav1.LabelSelector{
				MatchLabels: publicLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: publicLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "opni-alertmanager",
							Image:           "bitnami/alertmanager:latest",
							ImagePullPolicy: "Always",
							Ports:           r.containerAlertManagerPorts(),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "opni-alertmanager-data",
									MountPath: "/var/lib/alertmanager/data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "opni-alertmanager-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "opni-alertmanager-data",
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "opni-alertmanager-data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(r.gw.Spec.Alerting.Storage),
							},
						},
					}},
			},
		},
	}
	ctrl.SetControllerReference(r.gw, deploy, r.client.Scheme())

	publicSvcLabels := publicLabels

	alertingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "opni-alerting",
			Namespace:   r.gw.Namespace,
			Labels:      publicSvcLabels,
			Annotations: r.gw.Spec.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     r.gw.Spec.Alerting.ServiceType,
			Selector: publicLabels,
			Ports:    r.serviceAlertManagerPorts(r.containerAlertManagerPorts()),
		},
	}
	ctrl.SetControllerReference(r.gw, alertingSvc, r.client.Scheme())

	return []resources.Resource{
		resources.Present(deploy),
		resources.Present(alertingSvc),
	}
}

func (r *Reconciler) numAlertingReplicas() int32 {
	return 3
}

func (r *Reconciler) containerAlertManagerPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "alerting-port",
			ContainerPort: 9093,
			Protocol:      "TCP",
		},
	}
}

func (r *Reconciler) serviceAlertManagerPorts(
	containerPorts []corev1.ContainerPort) []corev1.ServicePort {
	svcAMPorts := make([]corev1.ServicePort, len(containerPorts))
	for i, port := range containerPorts {
		svcAMPorts[i] = corev1.ServicePort{
			Name:       port.Name,
			Port:       port.ContainerPort,
			Protocol:   port.Protocol,
			TargetPort: intstr.FromString(port.Name),
		}
	}
	return svcAMPorts
}
