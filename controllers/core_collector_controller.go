package controllers

import (
	"context"

	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/collector"
	"github.com/rancher/opni/pkg/util/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CoreCollectorReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.opni.io,resources=collectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.opni.io,resources=collectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.opni.io,resources=collectors/finalizers,verbs=update
// +kubebuilder:rbac:groups=logging.opni.io,resources=collectorconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=logging.opni.io,resources=collectorconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=logging.opni.io,resources=collectorconfigs/finalizers,verbs=update

func (r *CoreCollectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	collectorInstance := &corev1beta1.Collector{}
	err := r.Get(ctx, req.NamespacedName, collectorInstance)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	CollectorClusterReconciler := collector.NewReconciler(ctx, collectorInstance, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	reconcilers := []resources.ComponentReconciler{
		CollectorClusterReconciler.Reconcile,
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
func (r *CoreCollectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1beta1.Collector{}).
		For(&opniloggingv1beta1.CollectorConfig{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
