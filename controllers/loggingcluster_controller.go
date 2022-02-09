package controllers

import (
	"context"

	opensearchv1beta1 "github.com/rancher/opni-opensearch-operator/api/v1beta1"
	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/multicluster/loggingcluster"
	"github.com/rancher/opni/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoggingClusterReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=opni.io,resources=downstreamclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=downstreamclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opni.io,resources=downstreamclusters/finalizers,verbs=update

func (r *LoggingClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	loggingCluster := &v2beta1.LoggingCluster{}
	err := r.Get(ctx, req.NamespacedName, loggingCluster)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	loggingClusterReconciler := loggingcluster.NewReconciler(ctx, loggingCluster, r.Client)

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
		For(&v2beta1.LoggingCluster{}).
		Owns(&opensearchv1beta1.OpensearchCluster{}).
		Complete(r)
}
