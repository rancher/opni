package controllers

import (
	"context"

	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/opniopensearch"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/apimachinery/pkg/runtime"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OpniOpensearchReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=opni.io,resources=opniopensearches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=opniopensearches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opni.io,resources=opniopensearches/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *OpniOpensearchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	opniOpensearch := &v1beta2.OpniOpensearch{}
	err := r.Get(ctx, req.NamespacedName, opniOpensearch)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	opniOpensearchReconciler, err := opniopensearch.NewReconciler(ctx, opniOpensearch, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	reconcilers := []resources.ComponentReconciler{
		opniOpensearchReconciler.Reconcile,
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
func (r *OpniOpensearchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta2.OpniOpensearch{}).
		Owns(&opsterv1.OpenSearchCluster{}).
		Owns(&v1beta2.MulticlusterRoleBinding{}).
		Complete(r)
}
