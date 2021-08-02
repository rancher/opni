package elastic

import (
	"github.com/rancher/opni/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

func (r *Reconciler) elasticWorkloads() []resources.Resource {
	master := r.elasticMasterWorkload()
	data := r.elasticDataWorkload()
	client := r.elasticClientWorkload()
	return []resources.Resource{
		master,
		data,
		client,
	}
}

func (r *Reconciler) elasticDataWorkload() resources.Resource {
	labels := resources.NewElasticLabels().
		WithRole(resources.ElasticDataRole)

	deployment := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opendistro-es-data",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &r.opniCluster.Spec.Elastic.Workloads.Client.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						initSysctlContainer(),
						fixMountContainer(),
					},
					Containers: []corev1.Container{
						r.elasticDataContainer(),
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "opendistro-es-config",
								},
							},
						},
					},
				},
			},
		},
	}

	// If we are using persistent storage, create a PVC. Otherwise, create a
	// emptyDir volume.
	if pvcTemplate := r.maybePVC(); pvcTemplate != nil {
		deployment.Spec.VolumeClaimTemplates =
			append(deployment.Spec.VolumeClaimTemplates, *pvcTemplate)
	} else {
		deployment.Spec.Template.Spec.Volumes =
			append(deployment.Spec.Template.Spec.Volumes,
				corev1.Volume{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			)
	}
	return resources.Present(deployment)
}

func (r *Reconciler) maybePVC() *corev1.PersistentVolumeClaim {
	// Set up defaults
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "data",
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
	}

	if p := r.opniCluster.Spec.Elastic.Persistence; p != nil {
		if !p.Enabled {
			return nil
		}
		if len(p.AccessModes) > 0 {
			pvc.Spec.AccessModes = p.AccessModes
		}
		if p.StorageClass != "" {
			pvc.Spec.StorageClassName = &p.StorageClass
		}
		if p.Request != "" {
			pvc.Spec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(p.Request),
			}
		}
	}

	return &pvc
}

func (r *Reconciler) elasticMasterWorkload() resources.Resource {
	labels := resources.NewElasticLabels().
		WithRole(resources.ElasticMasterRole)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opendistro-es-master",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &r.opniCluster.Spec.Elastic.Workloads.Master.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		},
	}

	return resources.Present(deployment)
}

func (r *Reconciler) elasticClientWorkload() resources.Resource {
	return resources.Present(nil)
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
			"chown -R 1000:1000 /usr/share/elasticsearch/data",
		},
		VolumeMounts: []corev1.VolumeMount{
			dataVolumeMount(),
		},
	}
}

func dataVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "data",
		MountPath: "/usr/share/elasticsearch/data",
	}
}

func configVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "config",
		MountPath: "/usr/share/elasticsearch/config/logging.yml",
		SubPath:   "logging.yml",
	}
}

func (r *Reconciler) elasticDataContainer() corev1.Container {
	return corev1.Container{
		Name:  "elasticsearch",
		Image: "", // todo
		Ports: []corev1.ContainerPort{
			containerPort(transportPort),
		},
		VolumeMounts: []corev1.VolumeMount{
			dataVolumeMount(),
			configVolumeMount(),
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
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_CHROOT"},
			},
		},
		Env: combineEnvVars(
			elasticContainerEnv,
			downwardsAPIEnv,
			elasticNodeTypeEnv("data"),
			r.javaOptsEnv(),
		),
	}
}
