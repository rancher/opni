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
	"fmt"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	"github.com/rancher/opni/apis/v1beta1"
	opnierrors "github.com/rancher/opni/pkg/errors"
	"github.com/rancher/opni/pkg/providers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LogAdapterReconciler reconciles a LogAdapter object
type LogAdapterReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=opni.io,resources=logadapters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=opni.io,resources=logadapters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=opni.io,resources=logadapters/finalizers,verbs=update
//+kubebuilder:rbac:groups=logging.opni.io,resources=loggings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete

func (r *LogAdapterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// lg := log.FromContext(ctx)
	// Look up the object from the request.
	logAdapter := v1beta1.LogAdapter{}
	err := r.Get(ctx, req.NamespacedName, &logAdapter)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Look up the referenced OpniCluster to make sure it exists.
	opniCluster := v1beta1.OpniCluster{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      logAdapter.Spec.OpniCluster.Name,
		Namespace: logAdapter.Spec.OpniCluster.Namespace,
	},
		&opniCluster); err != nil {
		logAdapter.Status.Phase = "Error"
		logAdapter.Status.Message = opnierrors.InvalidReference.Error()
		r.Status().Update(ctx, &logAdapter)
		return ctrl.Result{}, fmt.Errorf("%w: { name: %s, namespace: %s }",
			opnierrors.InvalidReference, logAdapter.Spec.OpniCluster.Name, logAdapter.Spec.OpniCluster.Namespace)
	}

	if len(logAdapter.OwnerReferences) == 0 {
		// Tie the LogAdapter to the corresponding OpniCluster.
		logAdapter.OwnerReferences = append(logAdapter.OwnerReferences,
			v1.OwnerReference{
				APIVersion: opniCluster.APIVersion,
				Kind:       opniCluster.Kind,
				Name:       opniCluster.Name,
				UID:        opniCluster.UID,
			})
		logAdapter.Status.Phase = "Initializing"
		logAdapter.Status.Message = "Configuring Owner References"
		return ctrl.Result{
			Requeue: true,
		}, r.Update(ctx, &logAdapter)
	}

	logAdapter.Status.Conditions = []string{}

	// Don't process the request if the log adapter is being deleted.
	if logAdapter.DeletionTimestamp != nil {
		logAdapter.Status.Phase = "Deleting"
		r.Status().Update(ctx, &logAdapter)
		return ctrl.Result{}, nil
	}

	result, err := providers.ReconcileLogAdapter(ctx, r, &logAdapter)
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
func (r *LogAdapterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.LogAdapter{}).
		Owns(&loggingv1beta1.Logging{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Complete(r)
}
