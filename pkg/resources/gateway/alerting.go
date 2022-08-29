package gateway

import (
	"bytes"
	"fmt"
	"path"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/rancher/opni/pkg/resources"
)

var (
	defaultAlertManagerTemplate = shared.DefaultAlertManager
)

func (r *Reconciler) alerting() []resources.Resource {
	// always create the alerting service, even if it points to nothing, so cortex can find it

	publicLabels := map[string]string{} // TODO define a set of meaningful labels for this service
	labelWithAlert := func(label map[string]string) map[string]string {
		label["app.kubernetes.io/name"] = "opni-alerting"
		return label
	}
	publicLabels = labelWithAlert(publicLabels)
	publicSvcLabels := publicLabels

	alertingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        shared.OperatorAlertingServiceName,
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

	if r.gw.Spec.Alerting == nil {
		// set some sensible defaults
		r.spec.Alerting = &corev1beta1.AlertingSpec{
			WebPort:     9093,
			Storage:     "500Mi",
			ServiceType: "ClusterIP",
			ConfigName:  "alertmanager-config",
		}
	}

	// handle missing fields because the test suite is flaky locally
	if r.spec.Alerting.WebPort == 0 {
		r.spec.Alerting.WebPort = 9093
	}

	if r.gw.Spec.Alerting.Storage == "" {
		r.gw.Spec.Alerting.Storage = "500Mi"
	}
	if r.spec.Alerting.ConfigName == "" {
		r.spec.Alerting.ConfigName = "alertmanager-config"
	}

	var amData bytes.Buffer
	mgmtDNS := "opni-monitoring-internal"
	httpPort := "11080"
	err := defaultAlertManagerTemplate.Execute(&amData, shared.DefaultAlertManagerInfo{
		CortexHandlerName: shared.AlertingHookReceiverName,
		CortexHandlerURL:  fmt.Sprintf("https://%s:%s%s", mgmtDNS, httpPort, shared.AlertingCortexHookHandler),
	})
	if err != nil {
		panic(err)
	}

	// to be mounted into alertmanager pods
	alertManagerConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.spec.Alerting.ConfigName,
			Namespace: r.namespace,
		},

		Data: map[string]string{
			"alertmanager.yaml": amData.String(),
		},
	}
	ctrl.SetControllerReference(r.gw, alertManagerConfigMap, r.client.Scheme())

	dataMountPath := "/var/lib/alertmanager/data"
	configMountPath := "/etc/config"

	deploy := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting-internal",
			Namespace: r.namespace,
			Labels:    publicLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: lo.ToPtr(r.numAlertingReplicas()),
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
							// Defaults to
							// "--config.file=/opt/bitnami/alertmanager/conf/config.yml",
							// "--storage.path=/opt/bitnami/alertmanager/data"
							Args: []string{
								fmt.Sprintf("--config.file=%s", path.Join(configMountPath, "alertmanager.yaml")),
								fmt.Sprintf("--storage.path=%s", dataMountPath),
								"-p 9094:9094", // expose REST api port
							},
							Ports: r.containerAlertManagerPorts(),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "opni-alertmanager-data",
									MountPath: dataMountPath,
								},
								// volume mount for alertmanager configmap
								{
									Name:      "opni-alertmanager-config",
									MountPath: configMountPath,
									ReadOnly:  true,
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
						// mount alertmanager config map to volume
						{
							Name: "opni-alertmanager-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: r.spec.Alerting.ConfigName,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  "alertmanager.yaml",
											Path: "alertmanager.yaml",
										},
									},
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
								corev1.ResourceStorage: resource.MustParse(r.spec.Alerting.Storage),
							},
						},
					}},
			},
		},
	}
	ctrl.SetControllerReference(r.gw, deploy, r.client.Scheme())

	return []resources.Resource{
		resources.PresentIff(r.spec.Alerting.Enabled, alertManagerConfigMap),
		resources.PresentIff(r.spec.Alerting.Enabled, deploy),
		resources.PresentIff(r.spec.Alerting.Enabled, alertingSvc),
	}
}

func (r *Reconciler) numAlertingReplicas() int32 {
	return 3
}

func (r *Reconciler) containerAlertManagerPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "alert-web-port",
			ContainerPort: 9093,
			Protocol:      "TCP",
		},
		{
			Name:          "alert-api-port",
			ContainerPort: 9094,
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
