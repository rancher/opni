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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
	requestMapper := handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
		var collectorList corev1beta1.CollectorList
		if err := mgr.GetCache().List(context.Background(), &collectorList); err != nil {
			return nil
		}
		return reconcileRequestsForCollector(collectorList.Items, obj.GetName())
	})
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1beta1.Collector{}).
		Watches(&source.Kind{Type: &opniloggingv1beta1.CollectorConfig{}}, requestMapper).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func reconcileRequestsForCollector(collectors []corev1beta1.Collector, name string) (reqs []reconcile.Request) {
	for _, c := range collectors {
		if c.Spec.LoggingConfig != nil && c.Spec.LoggingConfig.Name == name {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: c.Namespace,
					Name:      c.Name,
				},
			})
		}
	}
	return
}
