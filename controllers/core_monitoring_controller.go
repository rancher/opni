//go:build !minimal

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/resources/monitoring"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// +kubebuilder:rbac:groups=core.opni.io,resources=monitoringclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.opni.io,resources=monitoringclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.opni.io,resources=monitoringclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

type CoreMonitoringReconciler struct {
	client.Client
	scheme *runtime.Scheme
	logger logr.Logger
}

func (r *CoreMonitoringReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := r.logger
	mc := &corev1beta1.MonitoringCluster{}
	err := r.Get(ctx, req.NamespacedName, mc)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	rec := monitoring.NewReconciler(ctx, r.Client, mc)
	result, err := rec.Reconcile()
	if err != nil {
		lg.WithValues(
			"gateway", mc.Name,
			"namespace", mc.Namespace,
		).Error(err, "failed to reconcile monitoring cluster")
		return ctrl.Result{}, err
	}
	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CoreMonitoringReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.logger = mgr.GetLogger().WithName("monitoring")
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1beta1.MonitoringCluster{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Watches(
			&corev1beta1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.findMonitoringClusters),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *CoreMonitoringReconciler) findMonitoringClusters(ctx context.Context, gw client.Object) []ctrl.Request {
	// Look up all MonitoringClusters that reference this gateway
	monitoringClusters := &corev1beta1.MonitoringClusterList{}
	err := r.List(ctx, monitoringClusters, client.InNamespace(gw.GetNamespace()))
	if err != nil {
		return []ctrl.Request{}
	}
	var requests []ctrl.Request
	for _, mc := range monitoringClusters.Items {
		if mc.Spec.Gateway.Name == gw.GetName() {
			requests = append(requests, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      mc.Name,
					Namespace: mc.Namespace,
				},
			})
		}
	}
	return requests
}
