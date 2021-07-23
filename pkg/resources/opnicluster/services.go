package opnicluster

import (
	"fmt"
	"strings"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/go-logr/logr"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) pretrainedModels() (resourceList []resources.Resource) {
	resourceList = []resources.Resource{}
	lg := logr.FromContext(r.ctx)
	requirement, err := labels.NewRequirement(
		PretrainedModelLabel,
		selection.Exists,
		[]string{},
	)
	if err != nil {
		panic(err)
	}
	deployments := appsv1.DeploymentList{}
	err = r.client.List(r.ctx, &deployments, &client.ListOptions{
		Namespace:     r.opniCluster.Namespace,
		LabelSelector: labels.NewSelector().Add(*requirement),
	})
	if err != nil {
		lg.Error(err, "failed to look up existing deployments")
		return
	}

	// Given a list of desired models, ensure the corresponding deployments exist.
	// If there are existing deployments that have been removed from the
	// list of desired models, mark them as deleted.
	models := map[string]*appsv1.Deployment{}
	desiredModels := r.opniCluster.Spec.Services.Inference.PretrainedModels
	for _, model := range desiredModels {
		models[model.Name] = nil
	}

	for _, deployment := range deployments.Items {
		d := deployment.DeepCopy()
		if _, ok := models[d.Name]; ok {
			// Model is still desired
			models[d.Name] = d
			resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
				return d, reconciler.StatePresent, nil
			})
		} else {
			// Model is no longer desired
			resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
				return d, reconciler.StateAbsent, nil
			})
		}
	}

	// Create new deployments for any that don't exist yet
	for _, model := range desiredModels {
		if _, ok := models[model.Name]; !ok {
			deployment, err := r.pretrainedModelDeployment(model)
			if err != nil {
				lg.Error(err, "failed to create pretrained model deployment")
				continue
			}
			resourceList = append(resourceList, deployment)
		}
	}
	return
}

func (r *Reconciler) pretrainedModelDeployment(
	modelRef v1beta1.PretrainedModelReference,
) (resources.Resource, error) {
	model, err := r.findPretrainedModel(modelRef)
	if err != nil {
		return nil, err
	}
	// Create a sidecar container either for downloading the model or copying it
	// directly from an image.
	var sidecar corev1.Container
	switch {
	case model.Spec.HTTP != nil:
		sidecar = httpSidecar(model)
	case model.Spec.Container != nil:
		sidecar = containerSidecar(model)
	default:
		return nil, fmt.Errorf(
			"Model %q is invalid. Must specify either HTTP or Container", modelRef.Name)
	}

	return func() (runtime.Object, reconciler.DesiredState, error) {
		labels := combineLabels(
			r.serviceLabels(v1beta1.InferenceService),
			r.pretrainedModelLabels(model.Name),
		)
		imageSpec := r.serviceImageSpec(v1beta1.InferenceService)
		lg := logr.FromContext(r.ctx)
		lg.Info("Creating pretrained model deployment", "name", model.Name)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("inference-service-%s", model.Name),
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{sidecar},
						Volumes: []corev1.Volume{
							{
								Name: "model-volume",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name:            "inference-service",
								Image:           *imageSpec.Image,
								ImagePullPolicy: *imageSpec.ImagePullPolicy,
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "model-volume",
										MountPath: "/model/",
									},
								},
							},
						},
						ImagePullSecrets: maybeImagePullSecrets(model),
					},
				},
			},
		}
		return deployment, reconciler.StatePresent, nil
	}, nil
}

func maybeImagePullSecrets(model v1beta1.PretrainedModel) []corev1.LocalObjectReference {
	if model.Spec.Container != nil {
		return model.Spec.Container.ImagePullSecrets
	}
	return nil
}

func httpSidecar(model v1beta1.PretrainedModel) corev1.Container {
	return corev1.Container{
		Name:  "download-model",
		Image: "docker.io/curlimages/curl:latest",
		Args: []string{
			"--silent",
			"--location",
			"--remote-name",
			"--output-dir", "/model/",
			"--url", model.Spec.HTTP.URL,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "model-volume",
				MountPath: "/model/",
			},
		},
	}
}

func containerSidecar(model v1beta1.PretrainedModel) corev1.Container {
	return corev1.Container{
		Name:    "copy-model",
		Image:   model.Spec.Container.Image,
		Command: []string{"/bin/sh"},
		Args:    strings.Fields(`-c cp /staging/* /model/ && chmod -R a+r /model/`),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "model-volume",
				MountPath: "/model/model.tar.gz",
				SubPath:   "model.tar.gz",
			},
		},
	}
}

func (r *Reconciler) findPretrainedModel(
	modelRef v1beta1.PretrainedModelReference,
) (v1beta1.PretrainedModel, error) {
	model := v1beta1.PretrainedModel{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      modelRef.Name,
		Namespace: r.opniCluster.Namespace,
	}, &model)
	if err != nil {
		return model, err
	}
	return model, nil
}

func (r *Reconciler) genericDeployment(service v1beta1.ServiceKind) *appsv1.Deployment {
	labels := r.serviceLabels(service)
	imageSpec := r.serviceImageSpec(service)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: labels[AppNameLabel],
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            labels[AppNameLabel],
							Image:           *imageSpec.Image,
							ImagePullPolicy: *imageSpec.ImagePullPolicy,
						},
					},
					ImagePullSecrets: imageSpec.ImagePullSecrets,
				},
			},
		},
	}
	return deployment
}

func (r *Reconciler) inferenceDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(InferenceLabel)
	deployment.Spec.Template.Spec.Containers[0].Resources =
		corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				"nvidia.com/gpu": resource.MustParse("1"),
			},
		}
	return deployment, reconciler.StatePresent, nil
}

func (r *Reconciler) drainDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(DrainLabel)
	return deployment, reconciler.StateCreated, nil
}

func (r *Reconciler) payloadReceiverDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(PayloadReceiverLabel)
	return deployment, reconciler.StatePresent, nil
}

func (r *Reconciler) preprocessingDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(PreprocessingLabel)
	return deployment, reconciler.StatePresent, nil
}
