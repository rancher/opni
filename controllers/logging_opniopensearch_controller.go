package controllers

import (
	"context"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/opniopensearch"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/apimachinery/pkg/runtime"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoggingOpniOpensearchReconciler struct {
	client.Client
	scheme *runtime.Scheme
	Opts   []opniopensearch.ReconcilerOption
}

// +kubebuilder:rbac:groups=logging.opni.io,resources=opniopensearches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=logging.opni.io,resources=opniopensearches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=logging.opni.io,resources=opniopensearches/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *LoggingOpniOpensearchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	opniOpensearch := &loggingv1beta1.OpniOpensearch{}
	err := r.Get(ctx, req.NamespacedName, opniOpensearch)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	opniOpensearchReconciler := opniopensearch.NewReconciler(ctx, opniOpensearch, r.Client, r.Opts...)

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
func (r *LoggingOpniOpensearchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&loggingv1beta1.OpniOpensearch{}).
		Owns(&opsterv1.OpenSearchCluster{}).
		Owns(&loggingv1beta1.MulticlusterRoleBinding{}).
		Complete(r)
}
