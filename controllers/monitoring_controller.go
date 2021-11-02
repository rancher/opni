package controllers

import (
	"fmt"

	monitoringctrl "github.com/banzaicloud/thanos-operator/controllers"
	thanosv1alpha1 "github.com/banzaicloud/thanos-operator/pkg/sdk/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=extensions;apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=extensions;networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;persistentvolumeclaims;events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=thanos.opni.io,resources=objectstores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=thanos.opni.io,resources=objectstores/status,verbs=get;update;patch

type ObjectStoreReconciler monitoringctrl.ObjectStoreReconciler

func (r *ObjectStoreReconciler) SetupWithManager(mgr ctrl.Manager) (controller.Controller, error) {
	r.Client = mgr.GetClient()
	r.Log = mgr.GetLogger().WithName("ObjectStore")
	return (*monitoringctrl.ObjectStoreReconciler)(r).SetupWithManager(mgr)
}

// +kubebuilder:rbac:groups=thanos.opni.io,resources=receivers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=thanos.opni.io,resources=receivers/status,verbs=get;update;patch

type ReceiverReconciler monitoringctrl.ReceiverReconciler

func (r *ReceiverReconciler) SetupWithManager(mgr ctrl.Manager) (controller.Controller, error) {
	r.Client = mgr.GetClient()
	r.Log = mgr.GetLogger().WithName("Receiver")
	return (*monitoringctrl.ReceiverReconciler)(r).SetupWithManager(mgr)
}

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

type ServiceMonitorWatchReconciler monitoringctrl.ServiceMonitorWatchReconciler

type ServiceMonitorWatchControllers struct {
	Receiver, ObjectStore, Thanos controller.Controller
}

func (r *ServiceMonitorWatchReconciler) SetupControllers(wc ServiceMonitorWatchControllers) {
	r.Controllers = map[string]monitoringctrl.ControllerWithSource{
		"receiver": {
			Controller: wc.Receiver,
			Source:     &thanosv1alpha1.Receiver{},
		},
		"objectstore": {
			Controller: wc.ObjectStore,
			Source:     &thanosv1alpha1.ObjectStore{},
		},
		"thanos": {
			Controller: wc.Thanos,
			Source:     &thanosv1alpha1.Thanos{},
		},
	}
}

func (r *ServiceMonitorWatchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.Log = mgr.GetLogger().WithName("ServiceMonitorWatch")
	return (*monitoringctrl.ServiceMonitorWatchReconciler)(r).SetupWithManager(mgr)
}

// +kubebuilder:rbac:groups=thanos.opni.io,resources=storeendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=thanos.opni.io,resources=storeendpoints/status,verbs=get;update;patch

type StoreEndpointReconciler monitoringctrl.StoreEndpointReconciler

func (r *StoreEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.Log = mgr.GetLogger().WithName("StoreEndpoint")
	return (*monitoringctrl.StoreEndpointReconciler)(r).SetupWithManager(mgr)
}

// +kubebuilder:rbac:groups=thanos.opni.io,resources=thanos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=thanos.opni.io,resources=thanos/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=*

type ThanosReconciler monitoringctrl.ThanosReconciler

func (r *ThanosReconciler) SetupWithManager(mgr ctrl.Manager) (controller.Controller, error) {
	r.Client = mgr.GetClient()
	r.Log = mgr.GetLogger().WithName("Thanos")
	return (*monitoringctrl.ThanosReconciler)(r).SetupWithManager(mgr)
}

// +kubebuilder:rbac:groups=thanos.opni.io,resources=thanosendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=thanos.opni.io,resources=thanosendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=integreatly.org,resources=grafanadatasources,verbs=get;list;watch;create;update;patch;delete

type ThanosEndpointReconciler monitoringctrl.ThanosEndpointReconciler

func (r *ThanosEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.Log = mgr.GetLogger().WithName("ThanosEndpoint")
	return (*monitoringctrl.ThanosEndpointReconciler)(r).SetupWithManager(mgr)
}

// +kubebuilder:rbac:groups=thanos.opni.io,resources=thanospeers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=thanos.opni.io,resources=thanospeers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=list;get;watch
// +kubebuilder:rbac:groups=integreatly.org,resources=grafanadatasources,verbs=get;list;watch;create;update;patch;delete

type ThanosPeerReconciler monitoringctrl.ThanosPeerReconciler

func (r *ThanosPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.Log = mgr.GetLogger().WithName("ThanosPeer")
	return (*monitoringctrl.ThanosPeerReconciler)(r).SetupWithManager(mgr)
}

type AggregateThanosReconcilers struct{}

func (r *AggregateThanosReconcilers) SetupWithManager(mgr ctrl.Manager) error {
	objectStoreRec := &ObjectStoreReconciler{}
	var receiverCtrl, objectStoreCtrl, thanosCtrl controller.Controller
	var err error
	if receiverCtrl, err = (&ReceiverReconciler{}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create Receiver controller: %w", err)
	}
	if objectStoreCtrl, err = objectStoreRec.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create ObjectStore controller: %w", err)
	}
	if thanosCtrl, err = (&ThanosReconciler{}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create Thanos controller: %w", err)
	}
	if err := (&StoreEndpointReconciler{}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create StoreEndpoint controller: %w", err)
	}
	if err := (&ThanosEndpointReconciler{}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create ThanosEndpoint controller: %w", err)
	}
	if err := (&ThanosPeerReconciler{}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create ThanosPeer controller: %w", err)
	}
	serviceMonitorWatchRec := &ServiceMonitorWatchReconciler{}
	serviceMonitorWatchRec.SetupControllers(ServiceMonitorWatchControllers{
		Receiver:    receiverCtrl,
		ObjectStore: objectStoreCtrl,
		Thanos:      thanosCtrl,
	})
	if err := serviceMonitorWatchRec.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create ServiceMonitorWatch controller: %w", err)
	}
	return nil
}
