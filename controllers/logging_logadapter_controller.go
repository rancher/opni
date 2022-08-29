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

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources/logadapter"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LogAdapterReconciler reconciles a LogAdapter object
type LoggingLogAdapterReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=logging.opni.io,resources=logadapters;opniclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=logging.opni.io,resources=logadapters;opniclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=logging.opni.io,resources=logadapters;opniclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=logging.opni.io,resources=*,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=policy,resources=podsecuritypolicies,verbs=get;list;watch;create;update;patch;delete;use
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=get;list;watch

func (r *LoggingLogAdapterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// lg := log.FromContext(ctx)
	// Look up the object from the request.
	logAdapter := opniloggingv1beta1.LogAdapter{}
	err := r.Get(ctx, req.NamespacedName, &logAdapter)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logAdapter.Status.Conditions = []string{}

	// Don't process the request if the log adapter is being deleted.
	if logAdapter.DeletionTimestamp != nil {
		logAdapter.Status.Phase = "Deleting"
		r.Status().Update(ctx, &logAdapter)
		return ctrl.Result{}, nil
	}

	result, err := logadapter.ReconcileLogAdapter(ctx, r, &logAdapter)
	if !result.IsZero() || err != nil {
		logAdapter.Status.Phase = "Processing"
		if err != nil {
			logAdapter.Status.Message = err.Error()
		}
		r.Status().Update(ctx, &logAdapter)
		return result, err
	}

	logAdapter.Status.Phase = "Ready"
	logAdapter.Status.Message = ""
	r.Status().Update(ctx, &logAdapter)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoggingLogAdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&opniloggingv1beta1.LogAdapter{}).
		Owns(&loggingv1beta1.Logging{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Complete(r)
}
