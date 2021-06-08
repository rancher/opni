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
	coreErrors "errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/opni/api/v1alpha1"
	"github.com/rancher/opni/pkg/demo"
)

// OpniDemoReconciler reconciles a OpniDemo object
type OpniDemoReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// KibanaDashboardPrerequisite describes a prerequisite object for the kibana dashboard pod
type KibanaDashboardPrerequisite struct {
	Name   string
	Object client.Object
}

// We need to give this controller all permissions due to it needing to install
// the helm controller, which itself needs all permissions
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

// /*+*/ kubebuilder:rbac:groups=demo.opni.io,resources=opnidemoes,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=demo.opni.io,resources=opnidemoes/status,verbs=get;update;patch
// /*+*/ kubebuilder:rbac:groups=demo.opni.io,resources=opnidemoes/finalizers,verbs=update
// /*+*/ kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=helm.cattle.io,resources=helmchartconfigs,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=helm.cattle.io,resources=helmcharts,verbs=get;list;watch;create;update;patch;delete
// /*+*/ kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *OpniDemoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("opnidemo", req.NamespacedName)

	opniDemo := &v1alpha1.OpniDemo{}
	if err := r.Get(ctx, req.NamespacedName, opniDemo); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Set conditions to empty to start
	opniDemo.Status.Conditions = []string{}

	if result, err := r.reconcileInfraStack(ctx, req, opniDemo); err != nil ||
		result.Requeue || result.RequeueAfter != time.Duration(0) {
		opniDemo.Status.State = "Deploying infrastructure resources"
		err = r.Status().Update(ctx, opniDemo)
		return result, err
	}

	if result, err := r.reconcileOpniStack(ctx, req, opniDemo); err != nil ||
		result.Requeue || result.RequeueAfter != time.Duration(0) {
		opniDemo.Status.State = "Deploying opni stack"
		err = r.Status().Update(ctx, opniDemo)
		return result, err
	}

	if result, err := r.reconcileServicesStack(ctx, req, opniDemo); err != nil ||
		result.Requeue || result.RequeueAfter != time.Duration(0) {
		opniDemo.Status.State = "Deploying services stack"
		err = r.Status().Update(ctx, opniDemo)
		return result, err
	}

	if result, err := r.reconcileKibanaDashboards(ctx, req, opniDemo); err != nil ||
		result.Requeue || result.RequeueAfter != time.Duration(0) {
		opniDemo.Status.State = "Deploying Kibana dashboard pod"
		err = r.Status().Update(ctx, opniDemo)
		return result, err
	}

	result := ctrl.Result{}
	if len(opniDemo.Status.Conditions) == 0 {
		opniDemo.Status.State = "Ready"
	} else {
		opniDemo.Status.State = "Waiting"
		result = ctrl.Result{
			RequeueAfter: 2 * time.Second,
		}
	}
	return result, r.Status().Update(ctx, opniDemo)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OpniDemoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.OpniDemo{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&rbacv1.Role{}).
		Owns(&extv1beta1.Ingress{}).
		Owns(&storagev1.StorageClass{}).
		Owns(&helmv1.HelmChart{}).
		Owns(&helmv1.HelmChartConfig{}).
		Complete(r)
}

func (r *OpniDemoReconciler) reconcileInfraStack(ctx context.Context, req ctrl.Request, opniDemo *v1alpha1.OpniDemo) (ctrl.Result, error) {
	objects := demo.MakeInfraStackObjects(opniDemo)
	for _, object := range objects {
		object.SetNamespace(opniDemo.Namespace)
		if err := r.Get(ctx, client.ObjectKeyFromObject(object), object); errors.IsNotFound(err) {
			r.Log.Info("creating resource", "name", client.ObjectKeyFromObject(object))
			if err := ctrl.SetControllerReference(opniDemo, object, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, r.Create(ctx, object)
		} else if err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *OpniDemoReconciler) reconcileOpniStack(ctx context.Context, req ctrl.Request, opniDemo *v1alpha1.OpniDemo) (ctrl.Result, error) {
	opts := opniDemo.Spec
	objects := []client.Object{}
	if opts.Components.Opni.Minio {
		objects = append(objects, demo.BuildMinioHelmChart(opniDemo))
	}
	if opts.Components.Opni.Nats {
		objects = append(objects, demo.BuildNatsHelmChart(opniDemo))
	}
	if opts.Components.Opni.Elastic {
		objects = append(objects, demo.BuildElasticHelmChart(opniDemo))
	}
	if opts.Components.Opni.RancherLogging {
		objects = append(objects, demo.BuildRancherLoggingCrdHelmChart(), demo.BuildRancherLoggingHelmChart())
	}
	if opts.Components.Opni.Traefik {
		objects = append(objects, demo.BuildTraefikHelmChart(opniDemo))
	}

	for _, object := range objects {
		object.SetNamespace(opniDemo.Namespace)
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: opniDemo.Namespace,
			Name:      object.GetName(),
		}, object); errors.IsNotFound(err) {
			r.Log.Info("creating resource", "name", client.ObjectKeyFromObject(object))
			if err := ctrl.SetControllerReference(opniDemo, object, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, r.Create(ctx, object)
		} else if err != nil {
			return ctrl.Result{}, err
		}
		if chart, ok := object.(*helmv1.HelmChart); ok {
			jobname := chart.Status.JobName
			job := &batchv1.Job{}
			if err := r.Get(ctx, types.NamespacedName{
				Namespace: opniDemo.Namespace,
				Name:      jobname,
			}, job); err != nil {
				opniDemo.Status.Conditions = append(opniDemo.Status.Conditions,
					fmt.Sprintf("Waiting for job %s to start (%s)", jobname, err.Error()))
			} else if job.Status.CompletionTime == nil {
				opniDemo.Status.Conditions = append(opniDemo.Status.Conditions,
					fmt.Sprintf("Waiting for job %s to complete", jobname))
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *OpniDemoReconciler) reconcileServicesStack(ctx context.Context, req ctrl.Request, opniDemo *v1alpha1.OpniDemo) (ctrl.Result, error) {
	objects := []client.Object{
		demo.BuildDrainService(opniDemo),
		demo.BuildNulogInferenceServiceControlPlane(opniDemo),
		demo.BuildPreprocessingService(opniDemo),
	}
	svc, dep, in := demo.BuildPayloadReceiverService(opniDemo)
	objects = append(objects, svc, dep, in)

	if !opniDemo.Spec.Quickstart {
		objects = append(objects,
			demo.BuildNulogInferenceService(opniDemo),
			demo.BuildNvidiaPlugin(opniDemo),
			demo.BuildTrainingController(opniDemo),
		)
		objects = append(objects, demo.BuildTrainingControllerInfra(opniDemo)...)
	}

	for _, object := range objects {
		object.SetNamespace(opniDemo.Namespace)
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: opniDemo.Namespace,
			Name:      object.GetName(),
		}, object); errors.IsNotFound(err) {
			r.Log.Info("creating resource", "name", client.ObjectKeyFromObject(object))
			if err := ctrl.SetControllerReference(opniDemo, object, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, r.Create(ctx, object)
		} else if err != nil {
			return ctrl.Result{}, err
		}
		switch o := object.(type) {
		case *appsv1.Deployment:
			if o.Status.AvailableReplicas < o.Status.Replicas || o.Status.UnavailableReplicas > 0 {
				opniDemo.Status.Conditions = append(opniDemo.Status.Conditions,
					fmt.Sprintf("Waiting for deployment %s to become ready", o.Name))
			}
		case *appsv1.DaemonSet:
			if o.Status.CurrentNumberScheduled != o.Status.DesiredNumberScheduled {
				opniDemo.Status.Conditions = append(opniDemo.Status.Conditions,
					fmt.Sprintf("Waiting for daemonset %s to become ready", o.Name))
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *OpniDemoReconciler) reconcileKibanaDashboards(ctx context.Context, req ctrl.Request, opniDemo *v1alpha1.OpniDemo) (ctrl.Result, error) {
	opts := opniDemo.Spec
	if opts.CreateKibanaDashboard != nil && !*opts.CreateKibanaDashboard {
		return ctrl.Result{}, nil
	}

	dashboardPrerequsites := [3]KibanaDashboardPrerequisite{
		{
			Name:   "opendistro-es-master",
			Object: &appsv1.StatefulSet{},
		},
		{
			Name:   "opendistro-es-data",
			Object: &appsv1.StatefulSet{},
		},
		{
			Name:   "opendistro-es-client",
			Object: &appsv1.Deployment{},
		},
	}
	for _, prerequisite := range dashboardPrerequsites {
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: opniDemo.Namespace,
			Name:      prerequisite.Name,
		}, prerequisite.Object); err != nil {
			return ctrl.Result{}, err
		}
		switch o := prerequisite.Object.(type) {
		case *appsv1.StatefulSet:
			if o.Status.ReadyReplicas < 1 {
				opniDemo.Status.Conditions = append(opniDemo.Status.Conditions,
					fmt.Sprintf("Waiting for prerequisite statefulset %s to become ready", o.Name))
				return ctrl.Result{RequeueAfter: time.Duration(2 * time.Second)}, nil
			}
		case *appsv1.Deployment:
			if o.Status.AvailableReplicas < 1 {
				opniDemo.Status.Conditions = append(opniDemo.Status.Conditions,
					fmt.Sprintf("Waiting for prerequisite deployment %s to become ready", o.Name))
				return ctrl.Result{RequeueAfter: time.Duration(2 * time.Second)}, nil
			}
		}
	}

	pod := demo.BuildKibanaDashboardPod(opniDemo)
	pod.SetNamespace(opniDemo.Namespace)
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: opniDemo.Namespace,
		Name:      demo.KibanaDashboardPodName,
	}, pod); errors.IsNotFound(err) {
		r.Log.Info("creating resource", "name", client.ObjectKeyFromObject(pod))
		if err := ctrl.SetControllerReference(opniDemo, pod, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, r.Create(ctx, pod)
	} else if err != nil {
		return ctrl.Result{}, err
	}
	switch s := pod.Status.Phase; s {
	case corev1.PodSucceeded:
		return ctrl.Result{}, nil
	case corev1.PodFailed:
		opniDemo.Status.Conditions = append(opniDemo.Status.Conditions,
			fmt.Sprintf("%s failed, deleting and rescheduling", pod.Name))
		if err := r.Delete(ctx, pod); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, coreErrors.New("kibana dashboard pod failed")
	default:
		opniDemo.Status.Conditions = append(opniDemo.Status.Conditions,
			fmt.Sprintf("Waiting for pod %s to finish, currently %s", pod.Name, s))
	}
	return ctrl.Result{}, nil
}
