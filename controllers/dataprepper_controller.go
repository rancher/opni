package controllers

import (
	"context"

	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/dataprepper"
	"github.com/rancher/opni/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DataPrepperReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=opni.io,resources=datapreppers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=datapreppers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opni.io,resources=datapreppers/finalizers,verbs=update

func (r *DataPrepperReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	DataPrepper := &v2beta1.DataPrepper{}
	err := r.Get(ctx, req.NamespacedName, DataPrepper)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	DataPrepperReconciler := dataprepper.NewReconciler(ctx, DataPrepper, r.Client)

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
func (r *DataPrepperReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2beta1.DataPrepper{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
