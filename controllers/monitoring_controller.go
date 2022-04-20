package controllers

import (
	"context"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources/monitoring"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=opni.io,resources=monitoringclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=monitoringclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opni.io,resources=monitoringclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=opni.io,resources=bootstraptokens,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=keyrings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=clusterissuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

type MonitoringReconciler struct {
	client.Client
	scheme *runtime.Scheme
	logger *zap.SugaredLogger
}

func (r *MonitoringReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := r.logger
	mc := &v1beta2.MonitoringCluster{}
	err := r.Get(ctx, req.NamespacedName, mc)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	rec := monitoring.NewReconciler(ctx, r.Client, mc)
	result, err := rec.Reconcile()
	if err != nil {
		lg.With(
			zap.String("gateway", mc.Name),
			zap.String("namespace", mc.Namespace),
			zap.Error(err),
		).Error("failed to reconcile monitoring cluster")
		return ctrl.Result{}, err
	}
	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MonitoringReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.logger = logger.New().Named("controller").Named("monitoring")
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta2.MonitoringCluster{}).
		Owns(&v1beta2.BootstrapToken{}).
		Owns(&v1beta2.Cluster{}).
		Owns(&v1beta2.Role{}).
		Owns(&cmv1.Certificate{}).
		Owns(&cmv1.Issuer{}).
		Owns(&cmv1.ClusterIssuer{}).
		Owns(&monitoringv1.ServiceMonitor{}).
		Owns(&v1beta2.RoleBinding{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
