//go:build !minimal

package controllers

import (
	grafanav1beta1 "github.com/grafana-operator/grafana-operator/v5/api/v1beta1"
	"log/slog"

	"github.com/go-logr/logr"
	grafanactrl "github.com/grafana-operator/grafana-operator/v5/controllers"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=grafana.opni.io,resources=grafanas;grafanas/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=grafana.opni.io,resources=grafanas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments;deployments/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps;secrets;serviceaccounts;services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

type GrafanaReconciler struct {
	client.Client
	Log    logr.Logger
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=grafana.opni.io,resources=grafanadashboards,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=grafana.opni.io,resources=grafanadashboards/status,verbs=get;update;patch

type GrafanaDashboardReconciler struct {
	client.Client
	Log    logr.Logger
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=grafana.opni.io,resources=grafanadatasources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=grafana.opni.io,resources=grafanadatasources/status,verbs=get;update;patch

type GrafanaDatasourceReconciler struct {
	client.Client
	Log    logr.Logger
	scheme *runtime.Scheme
}

func (r *GrafanaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	r.Log = mgr.GetLogger().WithName("controllers").WithName("Grafana").V(int(slog.LevelWarn))

	gc := &grafanactrl.GrafanaReconciler{
		Client: r.Client,
		Scheme: r.scheme,
		Log:    r.Log,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&grafanav1beta1.Grafana{}).
		Owns(&v1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(gc)
}

func (r *GrafanaDashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	r.Log = mgr.GetLogger().WithName("controllers").WithName("GrafanaDashboard").V(int(slog.LevelWarn))

	gc := grafanactrl.GrafanaDashboardReconciler{
		Client: r.Client,
		Scheme: r.scheme,
		Log:    r.Log,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&grafanav1beta1.GrafanaDashboard{}).
		Complete(&gc)
}

func (r *GrafanaDatasourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	r.Log = mgr.GetLogger().WithName("controllers").WithName("GrafanaDatasource").V(int(slog.LevelWarn))

	gc := &grafanactrl.GrafanaDatasourceReconciler{
		Client: r.Client,
		Scheme: r.scheme,
		Log:    r.Log,
	}

	//return gc.SetupWithManager(mgr, context.Background())
	return ctrl.NewControllerManagedBy(mgr).
		For(&grafanav1beta1.GrafanaDatasource{}).
		Complete(gc)
}
