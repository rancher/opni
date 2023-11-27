//go:build !minimal

package controllers

import (
	"context"

	opsterv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/multiclusteruser"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type LoggingMulticlusterUserReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=opni.io,resources=multiclusterusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=multiclusterusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opni.io,resources=multiclusterusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *LoggingMulticlusterUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	multiclusterUser := &loggingv1beta1.MulticlusterUser{}
	err := r.Get(ctx, req.NamespacedName, multiclusterUser)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add finalizer to resource if it's not deleted
	if multiclusterUser.DeletionTimestamp == nil {
		controllerutil.AddFinalizer(multiclusterUser, meta.OpensearchFinalizer)
		err = r.Update(ctx, multiclusterUser)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	multiclusterUserReconciler := multiclusteruser.NewReconciler(ctx, multiclusterUser, r.Client)
	reconcilers := []resources.ComponentReconciler{
		multiclusterUserReconciler.Reconcile,
	}

	for _, rec := range reconcilers {
		op := k8sutil.LoadResult(rec())
		if op.ShouldRequeue() {
			return op.Result()
		}
	}

	return k8sutil.DoNotRequeue().Result()
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoggingMulticlusterUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&loggingv1beta1.MulticlusterUser{}).
		Owns(&opsterv1.OpenSearchCluster{}).
		Complete(r)
}
