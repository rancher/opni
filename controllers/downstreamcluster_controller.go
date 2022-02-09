package controllers

import (
	"context"

	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/multicluster/downstreamcluster"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DownstreamClusterReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=opni.io,resources=loggingclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=loggingclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opni.io,resources=loggingclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opni.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opni.io,resources=opensearchclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opni.io,resources=opensearchclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opni.io,resources=dashboards,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opensearch.opni.io,resources=dashboards/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opensearch.opni.io,resources=dashboards/finalizers,verbs=update

func (r *DownstreamClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	downstreamCluster := &v2beta1.DownstreamCluster{}
	err := r.Get(ctx, req.NamespacedName, downstreamCluster)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	downstreamClusterReconciler := downstreamcluster.NewReconciler(ctx, downstreamCluster, r.Client)

	reconcilers := []resources.ComponentReconciler{
		downstreamClusterReconciler.Reconcile,
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
func (r *DownstreamClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2beta1.DownstreamCluster{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
