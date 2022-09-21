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

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/pkg/resources/pretrainedmodel"
	"github.com/rancher/opni/pkg/util/k8sutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PretrainedModelReconciler reconciles a PretrainedModel object
type AIPretrainedModelReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ai.opni.io,resources=pretrainedmodels,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ai.opni.io,resources=pretrainedmodels/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ai.opni.io,resources=pretrainedmodels/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *AIPretrainedModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	model := &aiv1beta1.PretrainedModel{}
	err := r.Get(ctx, req.NamespacedName, model)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	rec, err := pretrainedmodel.NewReconciler(ctx, r, model,
		reconciler.WithEnableRecreateWorkload(),
		reconciler.WithScheme(r.scheme),
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	return k8sutil.LoadResult(rec.Reconcile()).Result()
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIPretrainedModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1beta1.PretrainedModel{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
