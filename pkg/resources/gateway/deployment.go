package gateway

import (
	"github.com/rancher/opni/pkg/resources"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (r *Reconciler) deployment() (resources.Resource, error) {
	labels := resources.NewGatewayLabels()

	publicPorts, err := r.publicContainerPorts()
	if err != nil {
		return nil, err
	}
	internalPorts, err := r.managementContainerPorts()
	if err != nil {
		return nil, err
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway",
			Namespace: r.namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: lo.ToPtr[int32](1),
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
							Name:            "gateway",
							Image:           r.statusImage(),
							ImagePullPolicy: r.statusImagePullPolicy(),
							Command:         []string{"opni"},
							Args:            []string{"gateway"},
							Env: func() []corev1.EnvVar {
								return append(r.spec.ExtraEnvVars, corev1.EnvVar{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								})
							}(),

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
							Ports: append(append([]corev1.ContainerPort{
								{
									Name:          "metrics",
									ContainerPort: 8086,
								},
							}, publicPorts...), internalPorts...),
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
									DefaultMode: lo.ToPtr[int32](0400),
								},
							},
						},
						{
							Name: "certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "opni-gateway-serving-cert",
									DefaultMode: lo.ToPtr[int32](0400),
								},
							},
						},
						{
							Name: "cortex-client-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "cortex-client-cert-keys",
									DefaultMode: lo.ToPtr[int32](0400),
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
									DefaultMode: lo.ToPtr[int32](0400),
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
									DefaultMode: lo.ToPtr[int32](0400),
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
									DefaultMode: lo.ToPtr[int32](0400),
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
					NodeSelector:       r.spec.NodeSelector,
					Affinity:           r.spec.Affinity,
					Tolerations:        r.spec.Tolerations,
					ServiceAccountName: "opni-monitoring",
				},
			},
		},
	}

	for _, extraVol := range r.spec.ExtraVolumeMounts {
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
	// add additional volumes for alerting
	if r.spec.Alerting != nil && r.spec.Alerting.GatewayVolumeMounts != nil {
		for _, alertVol := range r.spec.Alerting.GatewayVolumeMounts {
			vol := corev1.Volume{
				Name:         alertVol.Name,
				VolumeSource: alertVol.VolumeSource,
			}
			volMount := corev1.VolumeMount{
				Name:      alertVol.Name,
				MountPath: alertVol.MountPath,
			}
			dep.Spec.Template.Spec.Volumes = append(dep.Spec.Template.Spec.Volumes, vol)
			dep.Spec.Template.Spec.Containers[0].VolumeMounts = append(dep.Spec.Template.Spec.Containers[0].VolumeMounts, volMount)
		}
	}

	r.setOwner(dep)
	return resources.Present(dep), nil
}
