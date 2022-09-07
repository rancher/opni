package controllers

import (
	"context"

	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/loggingcluster"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/meta"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type LoggingClusterReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=opni.io,resources=loggingclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=loggingclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opni.io,resources=loggingclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters/finalizers,verbs=update

func (r *LoggingClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	loggingCluster := &v1beta2.LoggingCluster{}
	err := r.Get(ctx, req.NamespacedName, loggingCluster)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add finalizer to resource if it's not deleted
	if loggingCluster.DeletionTimestamp == nil {
		controllerutil.AddFinalizer(loggingCluster, meta.OpensearchFinalizer)
		err = r.Update(ctx, loggingCluster)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	loggingClusterReconciler, err := loggingcluster.NewReconciler(ctx, loggingCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	reconcilers := []resources.ComponentReconciler{
		loggingClusterReconciler.Reconcile,
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
func (r *LoggingClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta2.LoggingCluster{}).
		Owns(&corev1.Secret{}).
		Owns(&opsterv1.OpenSearchCluster{}).
		Complete(r)
}
