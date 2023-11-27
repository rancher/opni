//go:build !minimal

package controllers

import (
	"context"

	opsterv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/multiclusterrolebinding"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

	multiclusterRoleBindingReconciler := multiclusterrolebinding.NewReconciler(ctx, multiclusterRoleBinding, r.Client)

	reconcilers := []resources.ComponentReconciler{
		multiclusterRoleBindingReconciler.Reconcile,
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
func (r *LoggingMulticlusterRoleBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	requestMapper := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		var mcrList loggingv1beta1.MulticlusterRoleBindingList
		if err := mgr.GetCache().List(ctx, &mcrList); err != nil {
			return nil
		}
		return reconcileRequestsForOpensearches(mcrList.Items, obj.GetName(), obj.GetNamespace())
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&loggingv1beta1.MulticlusterRoleBinding{}).
		Watches(&opsterv1.OpenSearchCluster{}, requestMapper).
		Complete(r)
}

func reconcileRequestsForOpensearches(
	mcrs []loggingv1beta1.MulticlusterRoleBinding,
	name string,
	namespace string,
) (reqs []reconcile.Request) {
	for _, mcr := range mcrs {
		if mcr.Spec.OpensearchCluster != nil &&
			mcr.Spec.OpensearchCluster.Name == name &&
			mcr.Spec.OpensearchCluster.Namespace == namespace {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: mcr.Namespace,
					Name:      mcr.Name,
				},
			})
		}
	}
	return
}
