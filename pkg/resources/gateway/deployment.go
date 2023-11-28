package gateway

import (
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/nats"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) deployment(extraAnnotations map[string]string) ([]resources.Resource, error) {
	labels := resources.NewGatewayLabels()

	publicPorts, err := r.publicContainerPorts()
	if err != nil {
		return nil, err
	}
	internalPorts, err := r.internalContainerPorts()
	if err != nil {
		return nil, err
	}
	adminDashboardPorts, err := r.adminDashboardContainerPorts()
	if err != nil {
		return nil, err
	}

	gatewayApiVersion := r.gw.APIVersion
	replicas := r.gw.Spec.Replicas
	if replicas == nil {
		replicas = lo.ToPtr[int32](3)
	}

	oldGatewayDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway",
			Namespace: r.gw.Namespace,
			Labels:    labels,
		},
	}

	gatewayStatefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway",
			Namespace: r.gw.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
				WhenScaled:  appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
			},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: extraAnnotations,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "gateway",
							Image:           r.gw.Status.Image,
							ImagePullPolicy: r.gw.Status.ImagePullPolicy,
							Command:         []string{"opni"},
							Args:            []string{"gateway"},
							Env: append(r.gw.Spec.ExtraEnvVars,
								corev1.EnvVar{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								corev1.EnvVar{
									Name: "POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								corev1.EnvVar{
									Name:  "GATEWAY_NAME",
									Value: r.gw.Name,
								},
								corev1.EnvVar{
									Name:  "GATEWAY_API_VERSION",
									Value: gatewayApiVersion,
								},
							),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "amtool",
									MountPath: "/etc/amtool",
								},
								{
									Name:      "certs",
									MountPath: "/run/opni/certs",
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
									Name:      "alerting-client-certs",
									MountPath: "/run/alerting/certs/client",
								},
								{
									Name:      "alerting-server-cacert",
									MountPath: "/run/alerting/certs/server",
								},
								{
									Name:      "etcd-client-certs",
									MountPath: "/run/etcd/certs/client",
								},
								{
									Name:      "etcd-server-cacert",
									MountPath: "/run/etcd/certs/server",
								},
								{
									Name:      "plugin-cache",
									MountPath: "/var/lib/opni/plugin-cache",
								},
								{
									Name:      "local-agent-key",
									MountPath: "/run/opni/keyring",
								},
							},
							Ports: append(append(publicPorts, internalPorts...), adminDashboardPorts...),
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromString("metrics"),
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
										Path: "/healthz",
										Port: intstr.FromString("metrics"),
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
										Path: "/healthz",
										Port: intstr.FromString("metrics"),
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
							Name: "amtool",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "amtool-config",
									},
									DefaultMode: lo.ToPtr[int32](0400),
								},
							},
						},
						{
							Name: "certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "opni-gateway-serving-cert",
									Items: []corev1.KeyToPath{
										{
											Key:  "tls.key",
											Path: "tls.key",
										},
									},
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
							Name: "alerting-client-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "alerting-client-cert-keys",
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
							Name: "alerting-server-cacert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "alerting-serving-cert-keys",
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
						{
							Name: "local-agent-key",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "opni-local-agent-key",
									DefaultMode: lo.ToPtr[int32](0400),
									Items: []corev1.KeyToPath{
										{
											Key:  "session-attribute.json",
											Path: "session-attribute.json",
										},
									},
								},
							},
						},
					},
					NodeSelector:       r.gw.Spec.NodeSelector,
					Affinity:           r.gw.Spec.Affinity,
					Tolerations:        r.gw.Spec.Tolerations,
					ServiceAccountName: "opni",
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "plugin-cache",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("8Gi"),
							},
						},
					},
				},
			},
		},
	}

	newEnvVars, newVolumeMounts, newVolumes := nats.ExternalNatsObjects(
		r.ctx,
		r.client,
		types.NamespacedName{
			Name:      r.gw.Spec.NatsRef.Name,
			Namespace: r.gw.Namespace,
		},
	)
	gatewayStatefulSet.Spec.Template.Spec.Volumes = append(gatewayStatefulSet.Spec.Template.Spec.Volumes, newVolumes...)
	gatewayStatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts = append(gatewayStatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, newVolumeMounts...)
	gatewayStatefulSet.Spec.Template.Spec.Containers[0].Env = append(gatewayStatefulSet.Spec.Template.Spec.Containers[0].Env, newEnvVars...)

	for _, extraVol := range r.gw.Spec.ExtraVolumeMounts {
		vol := corev1.Volume{
			Name:         extraVol.Name,
			VolumeSource: extraVol.VolumeSource,
		}
		volMount := corev1.VolumeMount{
			Name:      extraVol.Name,
			MountPath: extraVol.MountPath,
		}
		gatewayStatefulSet.Spec.Template.Spec.Volumes =
			append(gatewayStatefulSet.Spec.Template.Spec.Volumes, vol)
		gatewayStatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts =
			append(gatewayStatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, volMount)
	}

	// TODO(config)
	// // add additional volumes for alerting
	// if r.gw.Spec.Alerting.Enabled && r.gw.Spec.Alerting.GatewayVolumeMounts != nil {
	// 	for _, alertVol := range r.gw.Spec.Alerting.GatewayVolumeMounts {
	// 		vol := corev1.Volume{
	// 			Name:         alertVol.Name,
	// 			VolumeSource: alertVol.VolumeSource,
	// 		}
	// 		volMount := corev1.VolumeMount{
	// 			Name:      alertVol.Name,
	// 			MountPath: alertVol.MountPath,
	// 		}
	// 		gatewayStatefulSet.Spec.Template.Spec.Volumes = append(gatewayStatefulSet.Spec.Template.Spec.Volumes, vol)
	// 		gatewayStatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts = append(gatewayStatefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, volMount)
	// 	}
	// }

	ctrl.SetControllerReference(r.gw, gatewayStatefulSet, r.client.Scheme())
	return []resources.Resource{
		resources.Present(gatewayStatefulSet),
		resources.Absent(oldGatewayDeployment), // remove old deployment if it exists
	}, nil
}
