package controllers

import (
	"context"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/loggingclusterbinding"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/meta"
	"k8s.io/apimachinery/pkg/runtime"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type LoggingClusterBindingReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=logging.opni.io,resources=loggingclusterbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=logging.opni.io,resources=loggingclusterbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=logging.opni.io,resources=loggingclusterbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *LoggingClusterBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	loggingClusterBinding := &loggingv1beta1.LoggingClusterBinding{}
	err := r.Get(ctx, req.NamespacedName, loggingClusterBinding)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add finalizer to resource if it's not deleted
	if loggingClusterBinding.DeletionTimestamp == nil {
		controllerutil.AddFinalizer(loggingClusterBinding, meta.OpensearchFinalizer)
		err = r.Update(ctx, loggingClusterBinding)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	loggingClusterBindingReconciler := loggingclusterbinding.NewReconciler(ctx, loggingClusterBinding, r.Client)

	reconcilers := []resources.ComponentReconciler{
		loggingClusterBindingReconciler.Reconcile,
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
func (r *LoggingClusterBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&loggingv1beta1.LoggingClusterBinding{}).
		Owns(&opsterv1.OpenSearchCluster{}).
		Complete(r)
}
