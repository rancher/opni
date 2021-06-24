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
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1beta1 "github.com/rancher/opni/apis/v1beta1"
)

// OpniClusterReconciler reconciles a OpniCluster object
type OpniClusterReconciler struct {
	client.Client
	log    logr.Logger
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=opni.io,resources=opniclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opni.io,resources=opniclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opni.io,resources=opniclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete

func (r *OpniClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := r.log.WithValues("opnicluster", req.NamespacedName)

	// Ensure secrets are configured first

	opniCluster := &v1beta1.OpniCluster{}
	err := r.Get(ctx, req.NamespacedName, opniCluster)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	es := opniCluster.Spec.Elastic

	// Check if the elasticsearch credentials secret exists

	userSuppliedSecret := (es.Credentials.SecretRef != nil)
	esCredentials := &corev1.Secret{}

	if userSuppliedSecret {
		// The user has their own secret, make sure it exists
		ns := es.Credentials.SecretRef.Namespace
		if ns == "" {
			ns = opniCluster.Namespace
		}
		err = r.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      es.Credentials.SecretRef.Name,
		}, esCredentials)
		if errors.IsNotFound(err) {
			// The user's secret does not exist. Try again later
			lg.Error(err, "secret not found", "secret", esSecret.Name)
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, err
		}
	} else {
		// Use our own secret with the provided keys from the CR
		if es.Credentials.Keys == nil {
			lg.Error(err,
				"either keys or a secret reference must be specified",
				"field", "credentials",
			)
			// Do not requeue, the user has improperly configured the CR.
			return ctrl.Result{}, nil
		}

		// Check if it exists first
		err = r.Get(ctx, types.NamespacedName{
			Namespace: opniCluster.Namespace,
			Name:      esSecret.Name,
		}, esCredentials)

		if errors.IsNotFound(err) {
			// Keys are specified and the secret does not exist, create it now
			esCredentials.StringData = map[string]string{
				"username": es.Credentials.Keys.AccessKey,
				"password": es.Credentials.Keys.SecretKey,
			}
			esCredentials.ObjectMeta.Name = esSecret.Name
			esCredentials.ObjectMeta.Namespace = opniCluster.Namespace
			if err := r.Create(ctx, esCredentials); err != nil {
				lg.Error(err, "could not create secret", "secret", esSecret.Name)
				return ctrl.Result{}, err
			}
			lg.Info("created secret", "secret", esSecret.Name)
			return ctrl.Result{
				Requeue: true,
			}, nil
		}

		needsUpdate := false
		// Check if the keys match
		if string(esCredentials.Data["username"]) != es.Credentials.Keys.AccessKey {
			needsUpdate = true
			esCredentials.StringData["username"] = es.Credentials.Keys.AccessKey
		}
		if string(esCredentials.Data["password"]) != es.Credentials.Keys.SecretKey {
			needsUpdate = true
			esCredentials.StringData["password"] = es.Credentials.Keys.SecretKey
		}

		// If one or both keys do not match, update the secret and requeue
		if needsUpdate {
			return ctrl.Result{
				Requeue: true,
			}, r.Update(ctx, esCredentials)
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OpniClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.log = mgr.GetLogger().WithName("controllers").WithName("OpniCluster")
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.OpniCluster{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
