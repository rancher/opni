package demo

import (
	"github.com/rancher/opni/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func MakeInfraStackObjects(spec *v1alpha1.OpniDemo) (objects []client.Object) {
	objects = []client.Object{}
	helmObjects := []client.Object{
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

	if spec.Spec.Components.Infra.HelmController {
		objects = append(objects, helmObjects...)
	}

	return
}

var waitForFirstConsumer = storagev1.VolumeBindingWaitForFirstConsumer
var deleteReclaimPolicy = corev1.PersistentVolumeReclaimDelete
