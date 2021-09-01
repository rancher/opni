package controllers

import (
	nvidiacontrollers "github.com/NVIDIA/gpu-operator/controllers"
	ctrl "sigs.k8s.io/controller-runtime"
)

// +kubebuilder:rbac:groups=nvidia.opni.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions;proxies,verbs=get;list;watch
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces;serviceaccounts;pods;services;services/finalizers;endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims;events;configmaps;secrets;nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets;replicasets;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrule,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=scheduling.k8s.io,resources=priorityclasses,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch

type ClusterPolicyReconciler nvidiacontrollers.ClusterPolicyReconciler

func (r *ClusterPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.Log = mgr.GetLogger().WithName("ClusterPolicy")
	r.Scheme = mgr.GetScheme()
	return (*nvidiacontrollers.ClusterPolicyReconciler)(r).SetupWithManager(mgr)
}
