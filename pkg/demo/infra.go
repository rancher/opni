package demo

import (
	"fmt"

	"github.com/rancher/opni/apis/demo/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildHelmControllerObjects(spec *v1alpha1.OpniDemo) (objects []client.Object) {
	return []client.Object{
		&apiextv1beta1.CustomResourceDefinition{
			TypeMeta: v1.TypeMeta{
				Kind:       "CustomResourceDefinition",
				APIVersion: "apiextensions.k8s.io/v1beta1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name: "helmcharts.helm.cattle.io",
			},
			Spec: apiextv1beta1.CustomResourceDefinitionSpec{
				Group:   "helm.cattle.io",
				Version: "v1",
				Names: apiextv1beta1.CustomResourceDefinitionNames{
					Plural:   "helmcharts",
					Singular: "helmchart",
					Kind:     "HelmChart",
				},
				Scope: "Namespaced",
				AdditionalPrinterColumns: []apiextv1beta1.CustomResourceColumnDefinition{
					{
						Name:        "Job",
						Type:        "string",
						Format:      "",
						Description: "Job associated with updates to this chart",
						Priority:    0,
						JSONPath:    ".status.jobName",
					},
					{
						Name:        "Chart",
						Type:        "string",
						Format:      "",
						Description: "Helm Chart name",
						Priority:    0,
						JSONPath:    ".spec.chart",
					},
					{
						Name:        "TargetNamespace",
						Type:        "string",
						Format:      "",
						Description: "Helm Chart target namespace",
						Priority:    0,
						JSONPath:    ".spec.targetNamespace",
					},
					{
						Name:        "Version",
						Type:        "string",
						Format:      "",
						Description: "Helm Chart version",
						Priority:    0,
						JSONPath:    ".spec.version",
					},
					{
						Name:        "Repo",
						Type:        "string",
						Format:      "",
						Description: "Helm Chart repository URL",
						Priority:    0,
						JSONPath:    ".spec.repo",
					},
					{
						Name:        "HelmVersion",
						Type:        "string",
						Format:      "",
						Description: "Helm version used to manage the selected chart",
						Priority:    0,
						JSONPath:    ".spec.helmVersion",
					},
					{
						Name:        "Bootstrap",
						Type:        "boolean",
						Format:      "",
						Description: "True if this is chart is needed to bootstrap the cluster",
						Priority:    0,
						JSONPath:    ".spec.bootstrap",
					},
				},
			},
		},
		&apiextv1beta1.CustomResourceDefinition{
			TypeMeta: v1.TypeMeta{
				Kind:       "CustomResourceDefinition",
				APIVersion: "apiextensions.k8s.io/v1beta1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name: "helmchartconfigs.helm.cattle.io",
			},
			Spec: apiextv1beta1.CustomResourceDefinitionSpec{
				Group:   "helm.cattle.io",
				Version: "v1",
				Names: apiextv1beta1.CustomResourceDefinitionNames{
					Plural:   "helmchartconfigs",
					Singular: "helmchartconfig",
					Kind:     "HelmChartConfig",
				},
				Scope: "Namespaced",
			},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "helm-controller",
				Namespace: spec.Namespace,
			},
		},
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: "helm-controller",
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"*"},
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: "helm-controller",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "helm-controller",
					Namespace: spec.Namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind:     "ClusterRole",
				Name:     "helm-controller",
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "helm-controller",
				Namespace: spec.Namespace,
				Labels: map[string]string{
					"app": "helm-controller",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "helm-controller",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "helm-controller",
						},
					},
					Spec: corev1.PodSpec{
						ServiceAccountName: "helm-controller",
						Containers: []corev1.Container{
							{
								Name:    "helm-controller",
								Image:   "rancher/helm-controller:v0.8.4",
								Command: []string{"helm-controller"},
							},
						},
					},
				},
			},
		},
	}
}

func BuildNvidiaPlugin(spec *v1alpha1.OpniDemo) *appsv1.DaemonSet {
	labels := map[string]string{
		"name": "nvidia-device-plugin-ds",
	}
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nvidia-device-plugin-daemonset",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"scheduler.alpha.kubernetes.io/critical-pod": "",
					},
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{
						{
							Key:      "CriticalAddonsOnly",
							Operator: corev1.TolerationOpExists,
						},
						{
							Key:      "nvidia.com/gpu",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					PriorityClassName: "system-node-critical",
					Containers: []corev1.Container{
						{
							Name:  "nvidia-device-plugin-ctr",
							Image: fmt.Sprintf("nvidia/k8s-device-plugin:%s", spec.Spec.NvidiaVersion),
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: pointer.BoolPtr(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{
										corev1.Capability("ALL"),
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "device-plugin",
									MountPath: "/var/lib/kubelet/device-plugins",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "device-plugin",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/kubelet/device-plugins",
								},
							},
						},
					},
				},
			},
		},
	}
}
