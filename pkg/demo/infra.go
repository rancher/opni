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
	localPathObjects := []client.Object{
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-path-provisioner-service-account",
				Namespace: spec.Namespace,
			},
		},
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: "local-path-provisioner-role",
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"nodes", "persistentvolumeclaims", "configmaps"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints", "persistentvolumes", "pods"},
					Verbs:     []string{"*"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"events"},
					Verbs:     []string{"create", "patch"},
				},
				{
					APIGroups: []string{"storage.k8s.io"},
					Resources: []string{"storageclasses"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: "local-path-provisioner-bind",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "local-path-provisioner-service-account",
					Namespace: spec.Namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     "local-path-provisioner-role",
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-path-provisioner",
				Namespace: spec.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "local-path-provisioner",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "local-path-provisioner",
						},
					},
					Spec: corev1.PodSpec{
						PriorityClassName:  "system-node-critical",
						ServiceAccountName: "local-path-provisioner-service-account",
						Tolerations: []corev1.Toleration{
							{
								Key:      "CriticalAddonsOnly",
								Operator: corev1.TolerationOpExists,
							},
							{
								Key:      "node-role.kubernetes.io/control-plane",
								Operator: corev1.TolerationOpExists,
								Effect:   corev1.TaintEffectNoExecute,
							},
							{
								Key:      "node-role.kubernetes.io/master",
								Operator: corev1.TolerationOpExists,
								Effect:   corev1.TaintEffectNoSchedule,
							},
						},
						Containers: []corev1.Container{
							{
								Name:            "local-path-provisioner",
								Image:           "rancher/local-path-provisioner:v0.0.19",
								ImagePullPolicy: corev1.PullIfNotPresent,
								Command: []string{
									"local-path-provisioner",
									"start",
									"--config",
									"/etc/config/config.json",
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "config-volume",
										MountPath: "/etc/config",
									},
								},
								Env: []corev1.EnvVar{
									{
										Name: "POD_NAMESPACE",
										ValueFrom: &corev1.EnvVarSource{
											FieldRef: &corev1.ObjectFieldSelector{
												FieldPath: "metadata.namespace",
											},
										},
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "config-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "local-path-config",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		&storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "local-path",
				Annotations: map[string]string{
					"storageclass.kubernetes.io/is-default-class": "true",
				},
			},
			Provisioner:       "rancher.io/local-path",
			VolumeBindingMode: &waitForFirstConsumer,
			ReclaimPolicy:     &deleteReclaimPolicy,
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-path-config",
				Namespace: spec.Namespace,
			},
			Data: map[string]string{
				"config.json": `{
  "nodePathMap":[
  {
    "node":"DEFAULT_PATH_FOR_NON_LISTED_NODES",
    "paths":["/opt/aiops"]
  }
  ]
}`,
				"setup": `#!/bin/sh
while getopts "m:s:p:" opt
do
    case $opt in
        p)
        absolutePath=$OPTARG
        ;;
        s)
        sizeInBytes=$OPTARG
        ;;
        m)
        volMode=$OPTARG
        ;;
    esac
done
mkdir -m 0777 -p ${absolutePath}
`,
				"teardown": `#!/bin/sh
while getopts "m:s:p:" opt
do
    case $opt in
        p)
        absolutePath=$OPTARG
        ;;
        s)
        sizeInBytes=$OPTARG
        ;;
        m)
        volMode=$OPTARG
        ;;
    esac
done
rm -rf ${absolutePath}`,
				"helperPod.yaml": `apiVersion: v1
kind: Pod
metadata:
  name: helper-pod
spec:
  containers:
  - name: helper-pod
    image: rancher/library-busybox:1.32.1
`,
			},
		},
	}

	if spec.Spec.Components.Infra.HelmController {
		objects = append(objects, helmObjects...)
	}

	if spec.Spec.Components.Infra.LocalPathProvisioner {
		objects = append(objects, localPathObjects...)
	}

	return
}

var waitForFirstConsumer = storagev1.VolumeBindingWaitForFirstConsumer
var deleteReclaimPolicy = corev1.PersistentVolumeReclaimDelete
