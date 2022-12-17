package apis

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	nodev1 "k8s.io/api/node/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
)

func init() {
	addSchemeBuilders(
		appsv1.AddToScheme,
		batchv1.AddToScheme,
		corev1.AddToScheme,
		networkingv1.AddToScheme,
		nodev1.AddToScheme,
		rbacv1.AddToScheme,
		storagev1.AddToScheme,
	)
}
