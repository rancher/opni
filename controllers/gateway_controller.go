package controllers

import (
	"context"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/rancher/opni/apis/monitoring/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources/gateway"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=monitoring.opni.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.opni.io,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.opni.io,resources=gateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=monitoring.opni.io,resources=bootstraptokens,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.opni.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.opni.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.opni.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

type GatewayReconciler struct {
	client.Client
	scheme *runtime.Scheme
	logger *zap.SugaredLogger
}

func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := r.logger
	gw := &v1beta1.Gateway{}
	err := r.Get(ctx, req.NamespacedName, gw)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	gwReconciler := gateway.NewReconciler(ctx, r.Client, gw)
	result, err := gwReconciler.Reconcile()
	if err != nil {
		lg.With(
			zap.String("gateway", gw.Name),
			zap.String("namespace", gw.Namespace),
			zap.Error(err),
		).Error("failed to reconcile gateway")
		return ctrl.Result{}, err
	}
	if result != nil {
		return *result, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.logger = logger.New().Named("controller")
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.Gateway{}).
		Owns(&v1beta1.BootstrapToken{}).
		Owns(&v1beta1.Cluster{}).
		Owns(&v1beta1.Role{}).
		Owns(&cmv1.Certificate{}).
		Owns(&cmv1.Issuer{}).
		Owns(&cmv1.ClusterIssuer{}).
		Owns(&v1beta1.RoleBinding{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
