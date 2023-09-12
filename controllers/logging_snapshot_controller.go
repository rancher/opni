package controllers

import (
	"context"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/snapshot"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type LoggingSnapshotReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=logging.opni.io,resources=snapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=logging.opni.io,resources=snapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=logging.opni.io,resources=snapshots/finalizers,verbs=update
// +kubebuilder:rbac:groups=logging.opni.io,resources=opensearchrepositories,verbs=get;list;watch
// +kubebuilder:rbac:groups=logging.opni.io,resources=opensearchrepositories/status,verbs=get
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *LoggingSnapshotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	snapshotObj := &loggingv1beta1.Snapshot{}
	err := r.Get(ctx, req.NamespacedName, snapshotObj)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	snapshotReconciler := snapshot.NewReconciler(ctx, snapshotObj, r.Client)

	reconcilers := []resources.ComponentReconciler{
		snapshotReconciler.Reconcile,
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
func (r *LoggingSnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	requestMapper := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		var snapshotList loggingv1beta1.SnapshotList
		if err := mgr.GetCache().List(ctx, &snapshotList); err != nil {
			return nil
		}
		return reconcileSnapshotRequestsForRepositories(snapshotList.Items, obj.GetName(), obj.GetNamespace())
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&loggingv1beta1.Snapshot{}).
		Watches(&loggingv1beta1.OpensearchRepository{}, requestMapper).
		Complete(r)
}

func reconcileSnapshotRequestsForRepositories(
	snapshots []loggingv1beta1.Snapshot,
	name string,
	namespace string,
) (reqs []reconcile.Request) {
	for _, s := range snapshots {
		if s.Spec.Repository.Name == name && s.Namespace == namespace {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: s.Namespace,
					Name:      s.Name,
				},
			})
		}
	}
	return
}
