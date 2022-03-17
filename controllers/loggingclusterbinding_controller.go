package controllers

import (
	"context"

	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/loggingclusterbinding"
	"github.com/rancher/opni/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoggingClusterBindingReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=opni.io,resources=loggingclusterbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=loggingclusterbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opni.io,resources=loggingclusterbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *LoggingClusterBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	loggingClusterBinding := &v2beta1.LoggingClusterBinding{}
	err := r.Get(ctx, req.NamespacedName, loggingClusterBinding)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	loggingClusterBindingReconciler := loggingclusterbinding.NewReconciler(ctx, loggingClusterBinding, r.Client)

	reconcilers := []resources.ComponentReconciler{
		loggingClusterBindingReconciler.Reconcile,
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
func (r *LoggingClusterBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2beta1.LoggingClusterBinding{}).
		Complete(r)
}
