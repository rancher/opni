package controllers

import (
	"context"

	"github.com/go-logr/logr"
	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	grafanaconfig "github.com/grafana-operator/grafana-operator/v4/controllers/config"
	"github.com/grafana-operator/grafana-operator/v4/controllers/grafana"
	"github.com/grafana-operator/grafana-operator/v4/controllers/grafanadashboard"
	"github.com/grafana-operator/grafana-operator/v4/controllers/grafanadatasource"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=grafana.opni.io,resources=grafanas;grafanas/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=grafana.opni.io,resources=grafanas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=extensions;apps,resources=deployments;deployments/finalizers,verbs=get;list;watch;create;update;patch;delete
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
	r.Log = mgr.GetLogger().WithName("controllers").WithName("Grafana")
	ctx, ca := context.WithCancel(context.Background())
	gc := &grafana.ReconcileGrafana{
		Client:   r.Client,
		Scheme:   r.scheme,
		Log:      r.Log,
		Plugins:  grafana.NewPluginsHelper(),
		Context:  ctx,
		Cancel:   ca,
		Config:   grafanaconfig.GetControllerConfig(),
		Recorder: mgr.GetEventRecorderFor("GrafanaDashboard"),
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&grafanav1alpha1.Grafana{}).
		Complete(gc)
}

func (r *GrafanaDashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	r.Log = mgr.GetLogger().WithName("controllers").WithName("GrafanaDashboard")
	gc := &grafanadashboard.GrafanaDashboardReconciler{
		Client: r.Client,
		Scheme: r.scheme,
		Log:    r.Log,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&grafanav1alpha1.GrafanaDashboard{}).
		Complete(gc)
}

func (r *GrafanaDatasourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	r.Log = mgr.GetLogger().WithName("controllers").WithName("GrafanaDatasource")
	ctx, ca := context.WithCancel(context.Background())
	gc := &grafanadatasource.GrafanaDatasourceReconciler{
		Client:   r.Client,
		Scheme:   r.scheme,
		Logger:   r.Log,
		Context:  ctx,
		Cancel:   ca,
		Recorder: mgr.GetEventRecorderFor("GrafanaDatasource"),
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&grafanav1alpha1.GrafanaDataSource{}).
		Complete(gc)
}
