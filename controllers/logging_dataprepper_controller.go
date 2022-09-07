package controllers

import (
	"context"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/dataprepper"
	"github.com/rancher/opni/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoggingDataPrepperReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=logging.opni.io,resources=datapreppers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=logging.opni.io,resources=datapreppers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=logging.opni.io,resources=datapreppers/finalizers,verbs=update

func (r *LoggingDataPrepperReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	DataPrepper := &loggingv1beta1.DataPrepper{}
	err := r.Get(ctx, req.NamespacedName, DataPrepper)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	DataPrepperReconciler, err := dataprepper.NewReconciler(ctx, DataPrepper, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	reconcilers := []resources.ComponentReconciler{
		DataPrepperReconciler.Reconcile,
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
func (r *LoggingDataPrepperReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&loggingv1beta1.DataPrepper{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
