package controllers

import (
	"context"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/opensearchrepository"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type LoggingOpensearchRepositoryReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=logging.opni.io,resources=opensearchrepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=logging.opni.io,resources=opensearchrepositories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=logging.opni.io,resources=opensearchrepositories/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *LoggingOpensearchRepositoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	opensearchRepository := &loggingv1beta1.OpensearchRepository{}
	err := r.Get(ctx, req.NamespacedName, opensearchRepository)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add finalizer to resource if it's not deleted
	if opensearchRepository.DeletionTimestamp == nil {
		controllerutil.AddFinalizer(opensearchRepository, meta.OpensearchFinalizer)
		err = r.Update(ctx, opensearchRepository)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	opensearchRepositoryReconciler := opensearchrepository.NewReconciler(ctx, opensearchRepository, r.Client)

	reconcilers := []resources.ComponentReconciler{
		opensearchRepositoryReconciler.Reconcile,
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
func (r *LoggingOpensearchRepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	requestMapper := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		var repoList loggingv1beta1.OpensearchRepositoryList
		if err := mgr.GetCache().List(ctx, &repoList); err != nil {
			return nil
		}
		return reconcileRepositoryRequestsForOpensearches(repoList.Items, obj.GetName(), obj.GetNamespace())
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&loggingv1beta1.OpensearchRepository{}).
		Watches(&opsterv1.OpenSearchCluster{}, requestMapper).
		Complete(r)
}

func reconcileRepositoryRequestsForOpensearches(
	repos []loggingv1beta1.OpensearchRepository,
	name string,
	namespace string,
) (reqs []reconcile.Request) {
	for _, repo := range repos {
		if repo.Spec.OpensearchClusterRef != nil &&
			repo.Spec.OpensearchClusterRef.Name == name &&
			repo.Spec.OpensearchClusterRef.Namespace == namespace {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: repo.Namespace,
					Name:      repo.Name,
				},
			})
		}
	}
	return
}
