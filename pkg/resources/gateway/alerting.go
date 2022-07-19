package gateway

import (
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
	publicLabels := resources.NewGatewayLabels()
	labelWithAlert := func(label map[string]string) map[string]string {
		label["app"] = "alertmanager"
		return label
	}
	publicLabels = labelWithAlert(publicLabels)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting-internal",
			Namespace: r.gw.Spec.Alerting.Namespace,
			Labels:    publicLabels,
		},
		Spec: appsv1.DeploymentSpec{
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
						},
					},
				},
			},
		},
	}
	ctrl.SetControllerReference(r.gw, deploy, r.client.Scheme())
	publicSvcLabels := publicLabels
	publicSvcLabels["service-type"] = "public"

	alertingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "opni-alerting",
			Namespace:   r.gw.Spec.Alerting.Namespace,
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

	alrtVlm := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alerting-volume",
			Namespace: r.gw.Spec.Alerting.Namespace,
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("500Mi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRecycle,
			StorageClassName:              "slow",
			MountOptions: []string{
				"hard",
				"nfsvers=4.1",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{
					Server: "172.17.0.2", //FIXME: sensible default?
					Path:   "/alerting",
				},
			},
		},
	}
	ctrl.SetControllerReference(r.gw, alrtVlm, r.client.Scheme())
	return []resources.Resource{
		resources.Present(deploy),
		resources.Present(alertingSvc),
		resources.Present(alrtVlm),
	}
}

func (r *Reconciler) numAlertingReplicas() int32 {
	return 3
}

func (r *Reconciler) containerAlertManagerPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "alertmanager-port",
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
