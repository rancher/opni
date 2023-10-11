//go:build !minimal

package controllers

import (
	"context"
	"log/slog"

	"github.com/go-logr/logr"
	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	grafanaconfig "github.com/grafana-operator/grafana-operator/v4/controllers/config"
	"github.com/grafana-operator/grafana-operator/v4/controllers/grafana"
	"github.com/grafana-operator/grafana-operator/v4/controllers/grafanadashboard"
	"github.com/grafana-operator/grafana-operator/v4/controllers/grafanadatasource"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
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
	ctx, ca := context.WithCancel(context.Background())
	gc := &grafana.ReconcileGrafana{
		Client:   r.Client,
		Scheme:   r.scheme,
		Plugins:  grafana.NewPluginsHelper(),
		Context:  ctx,
		Cancel:   ca,
		Config:   grafanaconfig.GetControllerConfig(),
		Log:      r.Log,
		Recorder: mgr.GetEventRecorderFor("GrafanaDashboard"),
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&grafanav1alpha1.Grafana{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Complete(gc)
}

func (r *GrafanaDashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	r.Log = mgr.GetLogger().WithName("controllers").WithName("GrafanaDashboard").V(int(slog.LevelWarn))

	reconciler := grafanadashboard.NewReconciler(mgr)
	reconciler.(*grafanadashboard.GrafanaDashboardReconciler).Log = r.Log

	return grafanadashboard.SetupWithManager(mgr, reconciler, "")
}

func (r *GrafanaDatasourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	r.Log = mgr.GetLogger().WithName("controllers").WithName("GrafanaDatasource").V(int(slog.LevelWarn))
	ctx, ca := context.WithCancel(context.Background())
	gc := &grafanadatasource.GrafanaDatasourceReconciler{
		Client:   r.Client,
		Scheme:   r.scheme,
		Logger:   r.Log,
		Context:  ctx,
		Cancel:   ca,
		Recorder: mgr.GetEventRecorderFor("GrafanaDatasource"),
	}
	return gc.SetupWithManager(mgr)
}
