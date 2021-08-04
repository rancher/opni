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

const (
	natsNkeyDir = "/etc/nkey"
)

func (r *Reconciler) pretrainedModels() (resourceList []resources.Resource, retError error) {
	resourceList = []resources.Resource{}
	lg := logr.FromContext(r.ctx)
	requirement, err := labels.NewRequirement(
		resources.PretrainedModelLabel,
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
		retError = err
		lg.Error(err, "failed to look up existing deployments")
		return
	}

	// Given a list of desired models, ensure the corresponding deployments exist.
	// If there are existing deployments that have been removed from the
	// list of desired models, mark them as deleted.
	models := map[string]v1beta1.PretrainedModelReference{}
	desiredModels := r.opniCluster.Spec.Services.Inference.PretrainedModels
	for _, model := range desiredModels {
		models[model.Name] = model
	}
	existing := map[string]struct{}{}

	for _, deployment := range deployments.Items {
		// If the deployment is not in the list of desired models, mark it as deleted.
		// Otherwise, simply append the deployment to the list of resources which
		// should be present.
		modelName := deployment.Labels[resources.PretrainedModelLabel]
		existing[modelName] = struct{}{}
		if _, ok := models[modelName]; !ok {
			lg.Info("deleting pretrained model deployment", "model", modelName)
			resourceList = append(resourceList, resources.Absent(deployment.DeepCopy()))
		} else {
			resourceList = append(resourceList, resources.Present(deployment.DeepCopy()))
		}
	}

	// Create new deployments for any that don't exist yet
	for k, v := range models {
		if _, ok := existing[k]; !ok {
			deployment, err := r.pretrainedModelDeployment(v)
			if err != nil {
				lg.Error(err, "failed to create pretrained model resource")
				retError = err
				continue
			}
			lg.Info("creating pretrained model deployment", "model", k)
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
		labels := resources.CombineLabels(
			r.serviceLabels(v1beta1.InferenceService),
			r.pretrainedModelLabels(model.Name),
		)
		imageSpec := r.serviceImageSpec(v1beta1.InferenceService)
		lg := logr.FromContext(r.ctx)
		lg.Info("Creating pretrained model deployment", "name", model.Name)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("opni-inference-%s", model.Name),
				Namespace: r.opniCluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: r.opniCluster.APIVersion,
						Kind:       r.opniCluster.Kind,
						Name:       r.opniCluster.Name,
						UID:        r.opniCluster.UID,
					},
				},
				Labels: labels,
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
								Image:           imageSpec.GetImage(),
								ImagePullPolicy: imageSpec.GetImagePullPolicy(),
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
		insertHyperparametersVolume(deployment, &model)
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
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	envVars := []corev1.EnvVar{
		{
			Name:  "NATS_URL",
			Value: fmt.Sprintf("nats://%s-nats-client.%s.svc:%d", r.opniCluster.Name, r.opniCluster.Namespace, natsDefaultClientPort),
		},
	}
	switch r.opniCluster.Spec.Nats.AuthMethod {
	case v1beta1.NatsAuthUsername:
		var username string
		if r.opniCluster.Spec.Nats.Username == "" {
			username = "nats-user"
		} else {
			username = r.opniCluster.Spec.Nats.Username
		}
		newEnvVars := []corev1.EnvVar{
			{
				Name:  "NATS_USERNAME",
				Value: username,
			},
			{
				Name: "NATS_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: r.opniCluster.Status.Auth.AuthSecretKeyRef,
				},
			},
		}
		envVars = append(envVars, newEnvVars...)
	case v1beta1.NatsAuthNkey:
		newVolumes := []corev1.Volume{
			{
				Name: "nkey",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: r.opniCluster.Status.Auth.AuthSecretKeyRef.Name,
					},
				},
			},
		}
		volumes = append(volumes, newVolumes...)
		newVolumeMounts := []corev1.VolumeMount{
			{
				Name:      "nkey",
				ReadOnly:  true,
				MountPath: natsNkeyDir,
			},
		}
		volumeMounts = append(volumeMounts, newVolumeMounts...)
		newEnvVars := []corev1.EnvVar{
			{
				Name:  "NKEY_USER_FILENAME",
				Value: fmt.Sprintf("%s/pubkey", natsNkeyDir),
			},
			{
				Name:  "NKEY_USER_FILENAME",
				Value: fmt.Sprintf("%s/seed", natsNkeyDir),
			},
		}
		envVars = append(envVars, newEnvVars...)
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels[resources.AppNameLabel],
			Namespace: r.opniCluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: r.opniCluster.APIVersion,
					Kind:       r.opniCluster.Kind,
					Name:       r.opniCluster.Name,
					UID:        r.opniCluster.UID,
				},
			},
			Labels: labels,
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
							Name:            labels[resources.AppNameLabel],
							Image:           *imageSpec.Image,
							ImagePullPolicy: imageSpec.GetImagePullPolicy(),
							Env:             envVars,
							VolumeMounts:    volumeMounts,
						},
					},
					ImagePullSecrets: imageSpec.ImagePullSecrets,
					Volumes:          volumes,
				},
			},
		},
	}
	return deployment
}

func (r *Reconciler) inferenceDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta1.InferenceService)
	deployment.Spec.Template.Spec.Containers[0].Resources =
		corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				"nvidia.com/gpu": resource.MustParse("1"),
			},
		}
	deployment.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType
	return deployment, reconciler.StatePresent, nil
}

func (r *Reconciler) drainDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta1.DrainService)
	return deployment, reconciler.StateCreated, nil
}

func (r *Reconciler) payloadReceiverDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta1.PayloadReceiverService)
	return deployment, reconciler.StatePresent, nil
}

func (r *Reconciler) preprocessingDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta1.PreprocessingService)
	return deployment, reconciler.StatePresent, nil
}

func insertHyperparametersVolume(deployment *appsv1.Deployment, model *v1beta1.PretrainedModel) {
	volume := corev1.Volume{
		Name: "hyperparameters",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("opni-%s-hyperparameters", model.Name),
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "hyperparameters.json",
						Path: "hyperparameters.json",
					},
				},
			},
		},
	}
	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, volume)
	for i, container := range deployment.Spec.Template.Spec.Containers {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "hyperparameters",
			MountPath: "/hyperparameters.json",
			SubPath:   "hyperparameters.json",
			ReadOnly:  true,
		})
		deployment.Spec.Template.Spec.Containers[i] = container
	}
}
