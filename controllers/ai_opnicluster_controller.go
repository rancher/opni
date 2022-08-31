/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/opnicluster"
	"github.com/rancher/opni/pkg/util"
	opsterv1 "opensearch.opster.io/api/v1"
)

// OpniClusterReconciler reconciles a OpniCluster object
type AIOpniClusterReconciler struct {
	client.Client
	recorder record.EventRecorder
	scheme   *runtime.Scheme
}

// +kubebuilder:rbac:groups=ai.opni.io,resources=opniclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ai.opni.io,resources=opniclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ai.opni.io,resources=opniclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheuses,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;update;patch

// Required for insights service
// +kubebuilder:rbac:groups=core,resources=namespaces;endpoints;pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments;replicasets;daemonsets;statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs;cronjobs,verbs=get;list;watch

func (r *AIOpniClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	opniCluster := &aiv1beta1.OpniCluster{}
	err := r.Get(ctx, req.NamespacedName, opniCluster)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	opniReconciler, err := opnicluster.NewReconciler(ctx, r, r.recorder, opniCluster,
		reconciler.WithEnableRecreateWorkload(),
		reconciler.WithScheme(r.scheme),
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// indicesReconciler, err := indices.NewReconciler(ctx, opniCluster, r)
	// if err != nil {
	// 	return ctrl.Result{}, err
	// }

	reconcilers := []resources.ComponentReconciler{
		opniReconciler.Reconcile,
		//indicesReconciler.Reconcile,
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
func (r *AIOpniClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	r.recorder = mgr.GetEventRecorderFor("opni-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1beta1.OpniCluster{}).
		Owns(&v1beta2.LogAdapter{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&opsterv1.OpenSearchCluster{}).
		Complete(r)
}
