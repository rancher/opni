package controllers

import (
	"context"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/recurringsnapshot"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type LoggingRecurringSnapshotReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=logging.opni.io,resources=recurringsnapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=logging.opni.io,resources=recurringsnapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=logging.opni.io,resources=recurringsnapshots/finalizers,verbs=update
// +kubebuilder:rbac:groups=logging.opni.io,resources=opensearchrepositories,verbs=get;list;watch
// +kubebuilder:rbac:groups=logging.opni.io,resources=opensearchrepositories/status,verbs=get
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *LoggingRecurringSnapshotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	recurringSnapshot := &loggingv1beta1.RecurringSnapshot{}
	err := r.Get(ctx, req.NamespacedName, recurringSnapshot)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add finalizer to resource if it's not deleted
	if recurringSnapshot.DeletionTimestamp == nil {
		controllerutil.AddFinalizer(recurringSnapshot, meta.OpensearchFinalizer)
		err = r.Update(ctx, recurringSnapshot)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	recurringSnapshotReconciler := recurringsnapshot.NewReconciler(ctx, recurringSnapshot, r.Client)

	reconcilers := []resources.ComponentReconciler{
		recurringSnapshotReconciler.Reconcile,
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
func (r *LoggingRecurringSnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	requestMapper := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		var recurringSnapshotList loggingv1beta1.RecurringSnapshotList
		if err := mgr.GetCache().List(ctx, &recurringSnapshotList); err != nil {
			return nil
		}
		return reconcileRecurringSnapshotRequestsForRepositories(recurringSnapshotList.Items, obj.GetName(), obj.GetNamespace())
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&loggingv1beta1.RecurringSnapshot{}).
		Watches(&loggingv1beta1.OpensearchRepository{}, requestMapper).
		Complete(r)
}

func reconcileRecurringSnapshotRequestsForRepositories(
	snapshots []loggingv1beta1.RecurringSnapshot,
	name string,
	namespace string,
) (reqs []reconcile.Request) {
	for _, s := range snapshots {
		if s.Spec.Snapshot.Repository.Name == name && s.Namespace == namespace {
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
