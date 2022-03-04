package gateway

import (
	"github.com/rancher/opni-monitoring/pkg/sdk/resources"
	"github.com/rancher/opni-monitoring/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) deployment() (resources.Resource, error) {
	labels := resources.Labels()

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.gateway.Name,
			Namespace: r.gateway.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Pointer[int32](1),
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
							Name:            r.gateway.Name,
							Image:           r.gateway.Spec.Image.ImageOrDefault("rancher/opni-gateway:latest"),
							ImagePullPolicy: r.gateway.Spec.Image.ImagePullPolicyOrDefault(),
							Args:            []string{"gateway"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/opni-monitoring",
								},
								{
									Name:      "certs",
									MountPath: "/run/opni-monitoring/certs",
								},
								{
									Name:      "cortex-client-certs",
									MountPath: "/run/cortex/certs/client",
								},
								{
									Name:      "cortex-server-cacert",
									MountPath: "/run/cortex/certs/server",
								},
								{
									Name:      "etcd-client-certs",
									MountPath: "/run/etcd/certs/client",
								},
								{
									Name:      "etcd-server-cacert",
									MountPath: "/run/etcd/certs/server",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
								},
								{
									Name:          "grpc",
									ContainerPort: 9090,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/healthz",
										Port:   intstr.FromString("http"),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								TimeoutSeconds:   1,
								PeriodSeconds:    10,
								SuccessThreshold: 1,
								FailureThreshold: 3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/healthz",
										Port:   intstr.FromString("http"),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								TimeoutSeconds:   1,
								PeriodSeconds:    10,
								SuccessThreshold: 1,
								FailureThreshold: 3,
							},

							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/healthz",
										Port:   intstr.FromString("http"),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								TimeoutSeconds:   1,
								PeriodSeconds:    10,
								SuccessThreshold: 1,
								FailureThreshold: 10,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "opni-gateway",
									},
									DefaultMode: util.Pointer[int32](0400),
								},
							},
						},
						{
							Name: "certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "opni-gateway-serving-cert",
									DefaultMode: util.Pointer[int32](0400),
								},
							},
						},
						{
							Name: "cortex-client-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "cortex-client-cert-keys",
									DefaultMode: util.Pointer[int32](0400),
									Items: []corev1.KeyToPath{
										{
											Key:  "tls.crt",
											Path: "tls.crt",
										},
										{
											Key:  "tls.key",
											Path: "tls.key",
										},
										{
											Key:  "ca.crt",
											Path: "ca.crt",
										},
									},
								},
							},
						},
						{
							Name: "cortex-server-cacert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "cortex-serving-cert-keys",
									DefaultMode: util.Pointer[int32](0400),
									Items: []corev1.KeyToPath{
										{
											Key:  "ca.crt",
											Path: "ca.crt",
										},
									},
								},
							},
						},
						{
							Name: "etcd-client-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "etcd-client-cert-keys",
									DefaultMode: util.Pointer[int32](0400),
									Items: []corev1.KeyToPath{
										{
											Key:  "tls.crt",
											Path: "tls.crt",
										},
										{
											Key:  "tls.key",
											Path: "tls.key",
										},
										{
											Key:  "ca.crt",
											Path: "ca.crt",
										},
									},
								},
							},
						},
						{
							Name: "etcd-server-cacert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "etcd-serving-cert-keys",
									DefaultMode: util.Pointer[int32](0400),
									Items: []corev1.KeyToPath{
										{
											Key:  "ca.crt",
											Path: "ca.crt",
										},
									},
								},
							},
						},
					},
					NodeSelector: r.gateway.Spec.NodeSelector,
					Affinity:     r.gateway.Spec.Affinity,
					Tolerations:  r.gateway.Spec.Tolerations,
				},
			},
		},
	}

	for _, extraVol := range r.gateway.Spec.ExtraVolumeMounts {
		vol := corev1.Volume{
			Name:         extraVol.Name,
			VolumeSource: extraVol.VolumeSource,
		}
		volMount := corev1.VolumeMount{
			Name:      extraVol.Name,
			MountPath: extraVol.MountPath,
		}
		dep.Spec.Template.Spec.Volumes =
			append(dep.Spec.Template.Spec.Volumes, vol)
		dep.Spec.Template.Spec.Containers[0].VolumeMounts =
			append(dep.Spec.Template.Spec.Containers[0].VolumeMounts, volMount)
	}

	ports, err := r.optionalContainerPorts()
	if err != nil {
		return nil, err
	}
	dep.Spec.Template.Spec.Containers[0].Ports =
		append(dep.Spec.Template.Spec.Containers[0].Ports, ports...)
	ctrl.SetControllerReference(r.gateway, dep, r.client.Scheme())
	return resources.Present(dep), nil
}
