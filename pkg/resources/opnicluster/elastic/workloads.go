package elastic

import (
	"fmt"

	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) elasticWorkloads() []resources.Resource {
	return []resources.Resource{
		r.elasticMasterWorkload(),
		r.elasticDataWorkload(),
		r.elasticClientWorkload(),
		r.elasticKibanaWorkload(),
	}
}

func (r *Reconciler) elasticDataWorkload() resources.Resource {
	labels := resources.NewElasticLabels().
		WithRole(v1beta1.ElasticDataRole)

	workload := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniDataWorkload,
			Namespace: r.opniCluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: r.opniCluster.Spec.Elastic.Workloads.Data.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.OnDeleteStatefulSetStrategyType,
			},
			Template: r.elasticPodTemplate(labels),
		},
	}

	workload.Spec.Template.Spec.Affinity =
		r.opniCluster.Spec.Elastic.Workloads.Data.Affinity
	if r.opniCluster.Spec.Elastic.Workloads.Data.Resources != nil {
		workload.Spec.Template.Spec.Containers[0].Resources =
			*r.opniCluster.Spec.Elastic.Workloads.Data.Resources
	}

	ctrl.SetControllerReference(r.opniCluster, workload, r.client.Scheme())
	r.configurePVC(workload)
	return resources.Present(workload)
}

func (r *Reconciler) elasticPodTemplate(
	labels resources.ElasticLabels,
) corev1.PodTemplateSpec {
	imageSpec := r.openDistroImageSpec()
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				initSysctlContainer(),
			},
			Containers: []corev1.Container{
				{
					Name:            "elasticsearch",
					Image:           imageSpec.GetImage(),
					ImagePullPolicy: imageSpec.GetImagePullPolicy(),
					Ports:           containerPortsForRole(labels.Role()),
					VolumeMounts: []corev1.VolumeMount{
						configVolumeMount(),
						internalusersVolumeMount(),
					},
					LivenessProbe: &corev1.Probe{
						InitialDelaySeconds: 60,
						PeriodSeconds:       10,
						Handler: corev1.Handler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.FromString("transport"),
							},
						},
					},
					ReadinessProbe: &corev1.Probe{
						InitialDelaySeconds: 60,
						PeriodSeconds:       30,
						Handler: corev1.Handler{
							Exec: &corev1.ExecAction{
								Command: []string{
									"/bin/bash",
									"-c",
									"curl -k -u admin:${ES_PASSWORD} --silent --fail https://localhost:9200",
								},
							},
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{"SYS_CHROOT"},
						},
					},
					Env: combineEnvVars(
						elasticContainerEnv,
						downwardsAPIEnv,
						r.elasticNodeTypeEnv(labels.Role()),
						r.zenMastersEnv(),
						r.esPasswordEnv(),
						r.javaOptsEnv(labels.Role()),
					),
				},
			},
			NodeSelector: r.elasticNodeSelector(labels.Role()),
			Tolerations:  r.elasticTolerations(labels.Role()),
			Volumes: []corev1.Volume{
				configVolume(),
				internalusersVolume(),
			},
			ImagePullSecrets: imageSpec.ImagePullSecrets,
		},
	}
}

func containerPortsForRole(role v1beta1.ElasticRole) []corev1.ContainerPort {
	switch role {
	case v1beta1.ElasticDataRole:
		return []corev1.ContainerPort{
			containerPort(transportPort),
		}
	case v1beta1.ElasticClientRole, v1beta1.ElasticMasterRole:
		return []corev1.ContainerPort{
			containerPort(httpPort),
			containerPort(transportPort),
			containerPort(metricsPort),
			containerPort(rcaPort),
		}
	case v1beta1.ElasticKibanaRole:
		return []corev1.ContainerPort{
			containerPort(kibanaPort),
		}
	default:
		return nil
	}
}

func (r *Reconciler) elasticMasterWorkload() resources.Resource {
	labels := resources.NewElasticLabels().
		WithRole(v1beta1.ElasticMasterRole)

	workload := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniMasterWorkload,
			Namespace: r.opniCluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: r.opniCluster.Spec.Elastic.Workloads.Master.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: func() *int32 {
						if r.opniCluster.Status.OpensearchState.Version == nil {
							return pointer.Int32(0)
						}
						if *r.opniCluster.Status.OpensearchState.Version == r.opniCluster.Spec.Elastic.Version {
							return pointer.Int32(0)
						}
						return r.opniCluster.Spec.Elastic.Workloads.Master.Replicas
					}(),
				},
			},
			Template: r.elasticPodTemplate(labels),
		},
	}

	workload.Spec.Template.Spec.Affinity =
		r.opniCluster.Spec.Elastic.Workloads.Master.Affinity
	if r.opniCluster.Spec.Elastic.Workloads.Master.Resources != nil {
		workload.Spec.Template.Spec.Containers[0].Resources =
			*r.opniCluster.Spec.Elastic.Workloads.Master.Resources
	}

	ctrl.SetControllerReference(r.opniCluster, workload, r.client.Scheme())
	r.configurePVC(workload)
	return resources.Present(workload)
}

func (r *Reconciler) configurePVC(workload *appsv1.StatefulSet) {
	// Insert the data volume into the pod template.
	workload.Spec.Template.Spec.InitContainers = append(
		workload.Spec.Template.Spec.InitContainers, fixMountContainer())
	workload.Spec.Template.Spec.Containers[0].VolumeMounts = append(
		workload.Spec.Template.Spec.Containers[0].VolumeMounts, dataVolumeMount())

	// Set up defaults
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opni-es-data",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}

	usePersistence := false
	if p := r.opniCluster.Spec.Elastic.Persistence; p != nil {
		if !p.Enabled {
			// Persistence disabled
			return
		}
		usePersistence = true
		if len(p.AccessModes) > 0 {
			pvc.Spec.AccessModes = p.AccessModes
		}
		pvc.Spec.StorageClassName = p.StorageClassName
		resourceRequest := p.Request
		if resourceRequest.IsZero() {
			resourceRequest = resource.MustParse("10Gi")
		}
		pvc.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: resourceRequest,
		}
	}

	// If we are using persistent storage, create a PVC. Otherwise, create an
	// emptyDir volume.
	if usePersistence {
		workload.Spec.VolumeClaimTemplates =
			append(workload.Spec.VolumeClaimTemplates, pvc)
		workload.Spec.Template.Spec.Volumes =
			append(workload.Spec.Template.Spec.Volumes,
				corev1.Volume{
					Name: OpniDataWorkload,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "opni-es-data",
						},
					},
				},
			)
	} else {
		workload.Spec.Template.Spec.Volumes =
			append(workload.Spec.Template.Spec.Volumes,
				corev1.Volume{
					Name: "opni-es-data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			)
	}
}

func (r *Reconciler) elasticClientWorkload() resources.Resource {
	labels := resources.NewElasticLabels().
		WithRole(v1beta1.ElasticClientRole)

	workload := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniClientWorkload,
			Namespace: r.opniCluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: r.opniCluster.Spec.Elastic.Workloads.Client.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: r.elasticPodTemplate(labels),
		},
	}

	workload.Spec.Template.Spec.Affinity =
		r.opniCluster.Spec.Elastic.Workloads.Client.Affinity
	if r.opniCluster.Spec.Elastic.Workloads.Client.Resources != nil {
		workload.Spec.Template.Spec.Containers[0].Resources =
			*r.opniCluster.Spec.Elastic.Workloads.Client.Resources
	}

	ctrl.SetControllerReference(r.opniCluster, workload, r.client.Scheme())
	return resources.Present(workload)
}

func (r *Reconciler) elasticKibanaWorkload() resources.Resource {
	labels := resources.NewElasticLabels().
		WithRole(v1beta1.ElasticKibanaRole)

	imageSpec := r.kibanaImageSpec()
	workload := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniKibanaWorkload,
			Namespace: r.opniCluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: r.opniCluster.Spec.Elastic.Workloads.Kibana.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Affinity: r.opniCluster.Spec.Elastic.Workloads.Kibana.Affinity,
					Containers: []corev1.Container{
						{
							Name:            OpniKibanaWorkload,
							Image:           imageSpec.GetImage(),
							ImagePullPolicy: imageSpec.GetImagePullPolicy(),
							Env:             kibanaEnv,
							ReadinessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 60,
								PeriodSeconds:       30,
								SuccessThreshold:    1,
								TimeoutSeconds:      10,
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/api/status",
										Port: intstr.FromInt(5601),
									},
								},
							},
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								TimeoutSeconds:      1,
								Handler: corev1.Handler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(5601),
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 5601,
								},
							},
						},
					},
					ImagePullSecrets: imageSpec.ImagePullSecrets,
					NodeSelector:     r.elasticNodeSelector(labels.Role()),
					Tolerations:      r.elasticTolerations(labels.Role()),
				},
			},
		},
	}
	if r.opniCluster.Spec.Elastic.Workloads.Kibana.Resources != nil {
		workload.Spec.Template.Spec.Containers[0].Resources =
			*r.opniCluster.Spec.Elastic.Workloads.Kibana.Resources
	}

	ctrl.SetControllerReference(r.opniCluster, workload, r.client.Scheme())
	return resources.Present(workload)
}

func initSysctlContainer() corev1.Container {
	return corev1.Container{
		Name:  "init-sysctl",
		Image: "busybox:1.27.2",
		Command: []string{
			"sysctl",
			"-w",
			"vm.max_map_count=262144",
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.Bool(true),
		},
	}
}

func fixMountContainer() corev1.Container {
	return corev1.Container{
		Name:  "fix-mount",
		Image: "busybox:1.27.2",
		Command: []string{
			"sh",
			"-c",
			"chown -R 1000:1000 /usr/share/opensearch/data",
		},
		VolumeMounts: []corev1.VolumeMount{
			dataVolumeMount(),
		},
	}
}

func dataVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "opni-es-data",
		MountPath: "/usr/share/opensearch/data",
	}
}

func configVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "config",
		MountPath: "/usr/share/opensearch/config/logging.yml",
		SubPath:   "logging.yml",
	}
}

func configVolume() corev1.Volume {
	return corev1.Volume{
		Name: "config",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: "opni-es-config",
			},
		},
	}
}

func internalusersVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "internalusers",
		MountPath: fmt.Sprintf("/usr/share/opensearch/plugins/opensearch-security/securityconfig/%s", internalUsersKey),
		SubPath:   internalUsersKey,
	}
}

func internalusersVolume() corev1.Volume {
	return corev1.Volume{
		Name: "internalusers",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: internalUsersSecretName,
			},
		},
	}
}

func (r *Reconciler) openDistroImageSpec() v1beta1.ImageSpec {
	return v1beta1.ImageResolver{
		Version:             r.opniCluster.Spec.Elastic.Version,
		ImageName:           "opensearch",
		DefaultRepo:         "docker.io/opensearchproject",
		DefaultRepoOverride: r.opniCluster.Spec.Elastic.DefaultRepo,
		ImageOverride:       r.opniCluster.Spec.Elastic.Image,
	}.Resolve()
}

func (r *Reconciler) kibanaImageSpec() v1beta1.ImageSpec {
	return v1beta1.ImageResolver{
		Version:             r.opniCluster.Spec.Elastic.Version,
		ImageName:           "opensearch-dashboards",
		DefaultRepo:         "docker.io/opensearchproject",
		DefaultRepoOverride: r.opniCluster.Spec.Elastic.DefaultRepo,
		ImageOverride:       r.opniCluster.Spec.Elastic.KibanaImage,
	}.Resolve()
}

func (r *Reconciler) elasticNodeSelector(role v1beta1.ElasticRole) map[string]string {
	if s := role.GetNodeSelector(r.opniCluster); len(s) > 0 {
		return s
	}
	return r.opniCluster.Spec.GlobalNodeSelector
}

func (r *Reconciler) elasticTolerations(role v1beta1.ElasticRole) []corev1.Toleration {
	return append(r.opniCluster.Spec.GlobalTolerations, role.GetTolerations(r.opniCluster)...)
}
