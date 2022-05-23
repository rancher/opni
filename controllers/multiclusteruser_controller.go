package controllers

import (
	"context"

	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/multiclusteruser"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/meta"
	"k8s.io/apimachinery/pkg/runtime"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type MulticlusterUserReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=opni.io,resources=multiclusterusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=multiclusterusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opni.io,resources=multiclusterusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *MulticlusterUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	multiclusterUser := &v1beta2.MulticlusterUser{}
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
		op := util.LoadResult(rec())
		if op.ShouldRequeue() {
			return op.Result()
		}
	}

	return util.DoNotRequeue().Result()
}

// SetupWithManager sets up the controller with the Manager.
func (r *MulticlusterUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta2.MulticlusterUser{}).
		Owns(&opsterv1.OpenSearchCluster{}).
		Complete(r)
}
