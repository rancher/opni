package controllers

import (
	"context"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/multiclusterrolebinding"
	"github.com/rancher/opni/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoggingMulticlusterRoleBindingReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=logging.opni.io,resources=multiclusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=logging.opni.io,resources=multiclusterrolebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=logging.opni.io,resources=multiclusterrolebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *LoggingMulticlusterRoleBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	multiclusterRoleBinding := &loggingv1beta1.MulticlusterRoleBinding{}
	err := r.Get(ctx, req.NamespacedName, multiclusterRoleBinding)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	multiclusterRoleBindingReconciler, err := multiclusterrolebinding.NewReconciler(ctx, multiclusterRoleBinding, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	reconcilers := []resources.ComponentReconciler{
		multiclusterRoleBindingReconciler.Reconcile,
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
func (r *LoggingMulticlusterRoleBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&loggingv1beta1.MulticlusterRoleBinding{}).
		Owns(&opsterv1.OpenSearchCluster{}).
		Complete(r)
}
