package controllers

import (
	"context"

	"github.com/go-logr/logr"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"

	"github.com/rancher/opni/pkg/resources/alerting"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// +kubebuilder:rbac:groups=core.opni.io,resources=alertingclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.opni.io,resources=alertingclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.opni.io,resources=alertingclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

type CoreAlertingReconciler struct {
	client.Client
	scheme *runtime.Scheme
	logger logr.Logger
}

func (r *CoreAlertingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := r.logger
	ac := &corev1beta1.AlertingCluster{}
	err := r.Get(ctx, req.NamespacedName, ac)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	rec := alerting.NewReconciler(ctx, r.Client, ac)
	result, err := rec.Reconcile()
	if err != nil {
		lg.WithValues(
			"gateway", ac.Name,
			"namespace", ac.Namespace,
		).Error(err, "failed to reconcile alerting cluster")
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *CoreAlertingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.logger = mgr.GetLogger().WithName("alerting")
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1beta1.AlertingCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Watches(
			&source.Kind{
				Type: &corev1beta1.Gateway{},
			},
			handler.EnqueueRequestsFromMapFunc(r.findAlertingClusters),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&source.Kind{
				Type: &corev1beta1.AlertingCluster{},
			},
			handler.EnqueueRequestsFromMapFunc(r.findAlertingClusters),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *CoreAlertingReconciler) findAlertingClusters(gw client.Object) []ctrl.Request {
	alertingClusters := &corev1beta1.AlertingClusterList{}
	err := r.List(context.Background(), alertingClusters, client.InNamespace(gw.GetNamespace()))
	if err != nil {
		return []ctrl.Request{}
	}
	var requests []ctrl.Request
	for _, ac := range alertingClusters.Items {
		if ac.Spec.Gateway.Name == gw.GetName() {
			requests = append(requests, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      ac.Name,
					Namespace: ac.Namespace,
				},
			})
		}
	}
	return requests

}
