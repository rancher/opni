package opnicluster

import (
	"crypto/sha1"
	"fmt"
	"net/url"

	"emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/hyperparameters"
	"github.com/rancher/opni/pkg/util/nats"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) opniServices() ([]resources.Resource, error) {
	return []resources.Resource{
		r.nulogHyperparameters,
		r.inferenceDeployment,
		r.drainDeployment,
		r.workloadDrainDeployment,
		r.payloadReceiverDeployment,
		r.payloadReceiverService,
		r.preprocessingDeployment,
		r.gpuCtrlDeployment,
		r.metricsDeployment,
		r.metricsService,
		r.metricsServiceMonitor,
		r.metricsPrometheusRule,
		r.opensearchUpdateDeployment,
	}, nil
}

func (r *Reconciler) pretrainedModels() (resourceList []resources.Resource, retError error) {
	resourceList = []resources.Resource{}
	lg, _ := logr.FromContext(r.ctx)
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
		Namespace:     r.instanceNamespace,
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
	models := map[string]corev1.LocalObjectReference{}
	desiredModels := r.spec.Services.Inference.PretrainedModels
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
			deployment, err := r.pretrainedModelDeployment(models[modelName])
			if err != nil {
				lg.Error(err, "failed to reconcile existing model")
				retError = err
				continue
			}
			resourceList = append(resourceList, deployment)
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
	modelRef corev1.LocalObjectReference,
) (resources.Resource, error) {
	obj, err := r.findPretrainedModel(modelRef)
	if err != nil {
		return nil, err
	}

	switch model := obj.(type) {
	case *v1beta2.PretrainedModel:
		// Create a sidecar container either for downloading the model or copying it
		// directly from an image.
		var sidecar corev1.Container
		switch {
		case model.Spec.HTTP != nil:
			sidecar = httpSidecar(model.Spec.HTTP.URL)
		case model.Spec.Container != nil:
			sidecar = containerSidecar(model.Spec.Container.Image)
		default:
			return nil, fmt.Errorf(
				"model %q is invalid. Must specify either HTTP or Container", modelRef.Name)
		}

		return func() (runtime.Object, reconciler.DesiredState, error) {
			labels := resources.CombineLabels(
				r.serviceLabels(v1beta2.InferenceService),
				r.pretrainedModelLabels(model.Name),
			)
			imageSpec := r.serviceImageSpec(v1beta2.InferenceService)
			lg, _ := logr.FromContext(r.ctx)
			lg.V(1).Info("generating pretrained model deployment", "name", model.Name)
			envVars, volumeMounts, volumes := r.genericEnvAndVolumes()
			s3EnvVars := r.s3EnvVars()
			envVars = append(envVars, s3EnvVars...)
			volumes = append(volumes, corev1.Volume{
				Name: "model-volume",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "model-volume",
				MountPath: "/model/",
			})
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("opni-inference-%s", model.Name),
					Namespace: r.instanceNamespace,
					Labels:    labels,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: labels,
					},
					Replicas: model.Spec.Replicas,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
							Annotations: map[string]string{
								"opni.io/hyperparameters": hyperparameters.GenerateHyperParametersHash(model.Spec.Hyperparameters),
							},
						},
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{sidecar},
							Volumes:        volumes,
							Containers: []corev1.Container{
								{
									Name:            "inference-service",
									Image:           imageSpec.GetImage(),
									ImagePullPolicy: imageSpec.GetImagePullPolicy(),
									VolumeMounts:    volumeMounts,
									Env:             envVars,
								},
							},
							ImagePullSecrets: maybeImagePullSecrets(model),
							Tolerations:      r.serviceTolerations(v1beta2.InferenceService),
							NodeSelector:     r.serviceNodeSelector(v1beta2.InferenceService),
						},
					},
				},
			}
			r.setOwner(deployment)
			insertHyperparametersVolume(deployment, model.Name)

			return deployment, reconciler.StatePresent, nil
		}, nil
	case *aiv1beta1.PretrainedModel:
		// Create a sidecar container either for downloading the model or copying it
		// directly from an image.
		var sidecar corev1.Container
		switch {
		case model.Spec.HTTP != nil:
			sidecar = httpSidecar(model.Spec.HTTP.URL)
		case model.Spec.Container != nil:
			sidecar = containerSidecar(model.Spec.Container.Image)
		default:
			return nil, fmt.Errorf(
				"model %q is invalid. Must specify either HTTP or Container", modelRef.Name)
		}

		return func() (runtime.Object, reconciler.DesiredState, error) {
			labels := resources.CombineLabels(
				r.serviceLabels(v1beta2.InferenceService),
				r.pretrainedModelLabels(model.Name),
			)
			imageSpec := r.serviceImageSpec(v1beta2.InferenceService)
			lg, _ := logr.FromContext(r.ctx)
			lg.V(1).Info("generating pretrained model deployment", "name", model.Name)
			envVars, volumeMounts, volumes := r.genericEnvAndVolumes()
			s3EnvVars := r.s3EnvVars()
			envVars = append(envVars, s3EnvVars...)
			volumes = append(volumes, corev1.Volume{
				Name: "model-volume",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "model-volume",
				MountPath: "/model/",
			})
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("opni-inference-%s", model.Name),
					Namespace: r.instanceNamespace,
					Labels:    labels,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: labels,
					},
					Replicas: model.Spec.Replicas,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
							Annotations: map[string]string{
								"opni.io/hyperparameters": hyperparameters.GenerateHyperParametersHash(model.Spec.Hyperparameters),
							},
						},
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{sidecar},
							Volumes:        volumes,
							Containers: []corev1.Container{
								{
									Name:            "inference-service",
									Image:           imageSpec.GetImage(),
									ImagePullPolicy: imageSpec.GetImagePullPolicy(),
									VolumeMounts:    volumeMounts,
									Env:             envVars,
								},
							},
							ImagePullSecrets: maybeImagePullSecrets(model),
							Tolerations:      r.serviceTolerations(v1beta2.InferenceService),
							NodeSelector:     r.serviceNodeSelector(v1beta2.InferenceService),
						},
					},
				},
			}
			r.setOwner(deployment)
			insertHyperparametersVolume(deployment, model.Name)

			return deployment, reconciler.StatePresent, nil
		}, nil
	default:
		panic("invalid instance type")
	}
}

func maybeImagePullSecrets(instance interface{}) []corev1.LocalObjectReference {
	switch model := instance.(type) {
	case *v1beta2.PretrainedModel:
		if model.Spec.Container != nil {
			return model.Spec.Container.ImagePullSecrets
		}
		return nil
	case *aiv1beta1.PretrainedModel:
		if model.Spec.Container != nil {
			return model.Spec.Container.ImagePullSecrets
		}
		return nil
	default:
		panic("invalid instance type")
	}
}

func httpSidecar(url string) corev1.Container {
	return corev1.Container{
		Name:  "download-model",
		Image: "docker.io/curlimages/curl:latest",
		Args: []string{
			"--silent",
			"--location",
			"--remote-name",
			"--output-dir", "/model/",
			"--url", url,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "model-volume",
				MountPath: "/model/",
			},
		},
	}
}

func containerSidecar(image string) corev1.Container {
	return corev1.Container{
		Name:    "copy-model",
		Image:   image,
		Command: []string{"/bin/sh"},
		Args: []string{
			"-c",
			`cp /staging/* /model/ && chmod -R a+r /model/`,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "model-volume",
				MountPath: "/model/",
			},
		},
	}
}

func (r *Reconciler) gpuWorkerContainer() corev1.Container {
	imageSpec := r.serviceImageSpec(v1beta2.InferenceService)
	envVars, volumeMounts, _ := r.genericEnvAndVolumes()
	envVars = append(envVars, r.s3EnvVars()...)
	envVars = append(envVars, []corev1.EnvVar{
		{
			Name:  "MODEL_THRESHOLD",
			Value: "0.5",
		},
		{
			Name:  "MIN_LOG_TOKENS",
			Value: "5",
		},
		{
			Name:  "IS_GPU_SERVICE",
			Value: "True",
		},
		{
			Name:  "NVIDIA_VISIBLE_DEVICES",
			Value: "all",
		},
		{
			Name:  "NVIDIA_DRIVER_CAPABILITIES",
			Value: "compute,utility",
		},
	}...)
	if r.spec.S3.NulogS3Bucket != "" {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "S3_BUCKET",
				Value: r.spec.S3.NulogS3Bucket,
			})
	}
	return corev1.Container{
		Name:            "gpu-service-worker",
		Image:           *imageSpec.Image,
		ImagePullPolicy: imageSpec.GetImagePullPolicy(),
		Env:             envVars,
		VolumeMounts:    volumeMounts,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				"nvidia.com/gpu": resource.MustParse("1"),
			},
		},
	}
}

func (r *Reconciler) findPretrainedModel(
	modelRef corev1.LocalObjectReference,
) (client.Object, error) {
	var model client.Object
	if r.opniCluster != nil {
		model = &v1beta2.PretrainedModel{}
	}
	if r.aiOpniCluster != nil {
		model = &aiv1beta1.PretrainedModel{}
	}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      modelRef.Name,
		Namespace: r.instanceNamespace,
	}, model)
	if err != nil {
		return model, err
	}
	return model, nil
}

func (r *Reconciler) genericDeployment(service v1beta2.ServiceKind) *appsv1.Deployment {
	labels := r.serviceLabels(service)
	imageSpec := r.serviceImageSpec(service)
	envVars, volumeMounts, volumes := r.genericEnvAndVolumes()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels[resources.AppNameLabel],
			Namespace: r.instanceNamespace,
			Labels:    labels,
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
					NodeSelector:     r.serviceNodeSelector(service),
					Tolerations:      r.serviceTolerations(service),
				},
			},
		},
	}

	r.setOwner(deployment)
	return deployment
}

func (r *Reconciler) genericEnvAndVolumes() (
	envVars []corev1.EnvVar,
	volumeMounts []corev1.VolumeMount,
	volumes []corev1.Volume,
) {
	envVars = append(envVars, corev1.EnvVar{
		Name: "NATS_SERVER_URL",
		Value: fmt.Sprintf("nats://%s-nats-client.%s.svc:%d",
			r.instanceName, r.instanceNamespace, natsDefaultClientPort),
	})
	if r.opniCluster != nil {
		switch r.spec.Nats.AuthMethod {
		case v1beta2.NatsAuthUsername:
			if r.opniCluster.Status.Auth.NatsAuthSecretKeyRef == nil {
				break
			}
			var username string
			if r.spec.Nats.Username == "" {
				username = "nats-user"
			} else {
				username = r.spec.Nats.Username
			}
			newEnvVars := []corev1.EnvVar{
				{
					Name:  "NATS_USERNAME",
					Value: username,
				},
				{
					Name: "NATS_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: r.opniCluster.Status.Auth.NatsAuthSecretKeyRef,
					},
				},
			}
			envVars = append(envVars, newEnvVars...)
		case v1beta2.NatsAuthNkey:
			if r.opniCluster.Status.Auth.NatsAuthSecretKeyRef == nil {
				break
			}
			newVolumes := []corev1.Volume{
				{
					Name: "nkey",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: r.opniCluster.Status.Auth.NatsAuthSecretKeyRef.Name,
						},
					},
				},
			}
			volumes = append(volumes, newVolumes...)
			newVolumeMounts := []corev1.VolumeMount{
				{
					Name:      "nkey",
					ReadOnly:  true,
					MountPath: nats.NkeyDir,
				},
			}
			volumeMounts = append(volumeMounts, newVolumeMounts...)
			newEnvVars := []corev1.EnvVar{
				{
					Name:  "NKEY_SEED_FILENAME",
					Value: fmt.Sprintf("%s/seed", nats.NkeyDir),
				},
			}
			envVars = append(envVars, newEnvVars...)
		}
	}
	if r.aiOpniCluster != nil {
		newEnvVars, newVolumeMounts, newVolumes := nats.ExternalNatsObjects(
			r.ctx,
			r.client,
			types.NamespacedName{
				Name:      r.aiOpniCluster.Spec.NatsRef.Name,
				Namespace: r.instanceNamespace,
			},
		)
		envVars = append(envVars, newEnvVars...)
		volumes = append(volumes, newVolumes...)
		volumeMounts = append(volumeMounts, newVolumeMounts...)
	}
	envVars = append(envVars, corev1.EnvVar{
		Name: "ES_ENDPOINT",
		Value: func() string {
			if r.opensearchCluster != nil {
				return fmt.Sprintf(
					"https://%s.%s.svc:9200",
					r.opensearchCluster.Spec.General.ServiceName,
					r.opensearchCluster.Namespace,
				)
			}
			return fmt.Sprintf("http://opni-es-client.%s.svc:9200", r.instanceNamespace)
		}(),
	}, corev1.EnvVar{
		Name: "ES_USERNAME",
		Value: func() string {
			if r.opensearchCluster != nil {
				user, _, _ := helpers.UsernameAndPassword(
					r.ctx,
					r.client,
					r.opensearchCluster,
				)
				return user
			}
			return "admin"
		}(),
	}, corev1.EnvVar{
		Name: "ES_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: func() *corev1.SecretKeySelector {
				if r.opniCluster != nil {
					return r.opniCluster.Status.Auth.OpensearchAuthSecretKeyRef
				}
				if r.aiOpniCluster != nil {
					return r.aiOpniCluster.Status.Auth.OpensearchAuthSecretKeyRef
				}
				return nil
			}(),
		},
	})
	return
}

func (r *Reconciler) s3EnvVars() (envVars []corev1.EnvVar) {
	// lg := logr.FromContext(r.ctx)
	if r.opniCluster != nil &&
		r.opniCluster.Status.Auth.S3AccessKey != nil &&
		r.opniCluster.Status.Auth.S3SecretKey != nil &&
		r.opniCluster.Status.Auth.S3Endpoint != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "S3_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: r.opniCluster.Status.Auth.S3AccessKey,
			},
		}, corev1.EnvVar{
			Name: "S3_SECRET_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: r.opniCluster.Status.Auth.S3SecretKey,
			},
		}, corev1.EnvVar{
			Name:  "S3_ENDPOINT",
			Value: r.opniCluster.Status.Auth.S3Endpoint,
		})
	}

	if r.aiOpniCluster != nil &&
		r.aiOpniCluster.Status.Auth.S3AccessKey != nil &&
		r.aiOpniCluster.Status.Auth.S3SecretKey != nil &&
		r.aiOpniCluster.Status.Auth.S3Endpoint != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "S3_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: r.aiOpniCluster.Status.Auth.S3AccessKey,
			},
		}, corev1.EnvVar{
			Name: "S3_SECRET_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: r.aiOpniCluster.Status.Auth.S3SecretKey,
			},
		}, corev1.EnvVar{
			Name:  "S3_ENDPOINT",
			Value: r.aiOpniCluster.Status.Auth.S3Endpoint,
		})
	}
	return envVars
}

func deploymentState(enabled *bool) reconciler.DesiredState {
	if enabled == nil || *enabled {
		return reconciler.StatePresent
	}
	return reconciler.StateAbsent
}

func (r *Reconciler) nulogHyperparameters() (runtime.Object, reconciler.DesiredState, error) {
	var data map[string]intstr.IntOrString
	if len(r.spec.NulogHyperparameters) > 0 {
		data = r.spec.NulogHyperparameters
	} else {
		data = map[string]intstr.IntOrString{
			"modelThreshold": intstr.FromString("0.5"),
			"minLogTokens":   intstr.FromInt(5),
		}
	}
	cm, err := hyperparameters.GenerateHyperparametersConfigMap("nulog", r.instanceNamespace, data)
	if err != nil {
		return nil, nil, err
	}
	cm.Labels["opni-service"] = "nulog"
	err = r.setOwner(&cm)
	return &cm, reconciler.StatePresent, err
}

func (r *Reconciler) inferenceDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta2.InferenceService)
	addCPUInferenceLabel(deployment)
	s3EnvVars := r.s3EnvVars()
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, s3EnvVars...)
	if r.spec.S3.NulogS3Bucket != "" {
		deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "S3_BUCKET",
				Value: r.spec.S3.NulogS3Bucket,
			})
	}
	insertHyperparametersVolume(deployment, "nulog")
	return deployment, deploymentState(r.spec.Services.Inference.Enabled), nil
}

func (r *Reconciler) drainDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta2.DrainService)
	// temporary
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env,
		corev1.EnvVar{
			Name:  "FAIL_KEYWORDS",
			Value: "fail,error,missing,unable",
		})
	s3EnvVars := r.s3EnvVars()
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, s3EnvVars...)
	if r.spec.S3.DrainS3Bucket != "" {
		deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "S3_BUCKET",
				Value: r.spec.S3.DrainS3Bucket,
			})
	}
	deployment.Spec.Replicas = r.spec.Services.Drain.Replicas
	return deployment, deploymentState(r.spec.Services.Drain.Enabled), nil
}

func (r *Reconciler) pretrainedDrainDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta2.DrainService)
	// temporary
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env,
		corev1.EnvVar{
			Name:  "FAIL_KEYWORDS",
			Value: "fail,error,missing,unable",
		})
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env,
		corev1.EnvVar{
			Name:  "IS_PRETRAINED_SERVICE",
			Value: "false",
		})
	s3EnvVars := r.s3EnvVars()
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, s3EnvVars...)
	if r.spec.S3.DrainS3Bucket != "" {
		deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "S3_BUCKET",
				Value: r.spec.S3.DrainS3Bucket,
			})
	}
	deployment.Spec.Replicas = r.spec.Services.Drain.Replicas
	return deployment, deploymentState(r.spec.Services.Drain.Enabled), nil
}

func (r *Reconciler) payloadReceiverDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta2.PayloadReceiverService)
	return deployment, deploymentState(r.spec.Services.PayloadReceiver.Enabled), nil
}

func (r *Reconciler) payloadReceiverService() (runtime.Object, reconciler.DesiredState, error) {
	labels := r.serviceLabels(v1beta2.PayloadReceiverService)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels[resources.AppNameLabel],
			Namespace: r.instanceNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port: 80,
				},
			},
		},
	}
	r.setOwner(service)
	return service, deploymentState(r.spec.Services.PayloadReceiver.Enabled), nil
}

func (r *Reconciler) preprocessingDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta2.PreprocessingService)
	deployment.Spec.Replicas = r.spec.Services.Preprocessing.Replicas
	return deployment, deploymentState(r.spec.Services.Preprocessing.Enabled), nil
}

func (r *Reconciler) gpuCtrlDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta2.GPUControllerService)
	dataVolume := corev1.Volume{
		Name: "data",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	dataVolumeMount := corev1.VolumeMount{
		Name:      "data",
		MountPath: "/var/opni-data",
	}
	deployment.Spec.Template.Spec.RuntimeClassName = r.spec.Services.GPUController.RuntimeClass
	if features.DefaultMutableFeatureGate.Enabled(features.GPUOperator) && r.spec.Services.GPUController.RuntimeClass == nil {
		deployment.Spec.Template.Spec.RuntimeClassName = lo.ToPtr("nvidia")
	}
	deployment.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType
	deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, r.gpuWorkerContainer())

	// TODO: workaround for clone3 seccomp issues - remove when fixed
	for i, container := range deployment.Spec.Template.Spec.Containers {
		container.SecurityContext = &corev1.SecurityContext{
			Privileged: lo.ToPtr(true),
		}
		deployment.Spec.Template.Spec.Containers[i] = container
	}

	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, dataVolume)
	for i, container := range deployment.Spec.Template.Spec.Containers {
		container.VolumeMounts = append(container.VolumeMounts, dataVolumeMount)
		deployment.Spec.Template.Spec.Containers[i] = container
	}
	insertHyperparametersVolume(deployment, "nulog")
	return deployment, deploymentState(r.spec.Services.GPUController.Enabled), nil
}

func (r *Reconciler) getPrometheusEndpoint() (endpoint string) {
	lg := log.FromContext(r.ctx)
	if r.spec.Services.Metrics.Enabled != nil && !*r.spec.Services.Metrics.Enabled {
		return
	}
	if r.spec.Services.Metrics.PrometheusReference != nil {
		prometheus := &monitoringv1.Prometheus{}
		err := r.client.Get(r.ctx, types.NamespacedName{
			Name:      r.spec.Services.Metrics.PrometheusReference.Name,
			Namespace: r.spec.Services.Metrics.PrometheusReference.Namespace,
		}, prometheus)
		if err != nil {
			lg.V(1).Error(err, "unable to fetch prometheus")
		} else {
			endpoint = prometheus.Spec.ExternalURL
		}
	}
	if r.spec.Services.Metrics.PrometheusEndpoint != "" {
		endpoint = r.spec.Services.Metrics.PrometheusEndpoint
	}

	return
}

func (r *Reconciler) metricsDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta2.MetricsService)
	prometheusEndpoint := r.getPrometheusEndpoint()
	_, err := url.ParseRequestURI(prometheusEndpoint)
	if err != nil && (r.spec.Services.Metrics.Enabled == nil || *r.spec.Services.Metrics.Enabled) {
		return deployment, deploymentState(r.spec.Services.Metrics.Enabled), errors.New("prometheus endpoint is not a valid URL")
	}
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  "PROMETHEUS_ENDPOINT",
		Value: prometheusEndpoint,
	})
	for _, extraVolume := range r.spec.Services.Metrics.ExtraVolumeMounts {
		deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{
			Name:         extraVolume.Name,
			VolumeSource: extraVolume.VolumeSource,
		})
		deployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(deployment.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      extraVolume.Name,
			ReadOnly:  extraVolume.ReadOnly,
			MountPath: extraVolume.MountPath,
		})
	}
	return deployment, deploymentState(r.spec.Services.Metrics.Enabled), nil
}

func (r *Reconciler) metricsService() (runtime.Object, reconciler.DesiredState, error) {
	labels := r.serviceLabels(v1beta2.MetricsService)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels[resources.AppNameLabel],
			Namespace: r.instanceNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Port:       8000,
					Name:       "metrics",
					TargetPort: intstr.FromInt(8000),
				},
			},
		},
	}
	r.setOwner(service)
	return service, deploymentState(r.spec.Services.Metrics.Enabled), nil
}

func (r *Reconciler) metricsServiceMonitor() (runtime.Object, reconciler.DesiredState, error) {
	labels := r.serviceLabels(v1beta2.MetricsService)
	serviceMonitor := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels[resources.AppNameLabel],
			Namespace: r.instanceNamespace,
			Labels:    labels,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port:          "metrics",
					ScrapeTimeout: "5s",
				},
			},
		},
	}
	r.setOwner(serviceMonitor)
	if r.spec.Services.Metrics.PrometheusReference == nil {
		return serviceMonitor, reconciler.StateAbsent, nil
	}
	return serviceMonitor, deploymentState(r.spec.Services.Metrics.Enabled), nil
}

func (r *Reconciler) metricsPrometheusRule() (runtime.Object, reconciler.DesiredState, error) {
	labels := r.serviceLabels(v1beta2.MetricsService)
	prometheusRule := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", labels[resources.AppNameLabel], r.generateSHAID()),
			Namespace: r.instanceNamespace,
			Labels:    labels,
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name: "opni-rules",
					Rules: []monitoringv1.Rule{
						{
							Alert: "ClusterCpuUsageAnomalous",
							Expr:  intstr.FromString(`cpu_usage{value_type="is_alert"} >= 1`),
							Annotations: map[string]string{
								"summary":     "Anomalous CPU usage in cluster",
								"description": "Opni has calculated an alert score >= 5 for cluster CPU usage",
							},
						},
						{
							Alert: "ClusterMemoryUsageAnomalous",
							Expr:  intstr.FromString(`memory_usage{value_type="is_alert"} >= 1`),
							Annotations: map[string]string{
								"summary":     "Anomalous memory usage in cluster",
								"description": "Opni has calculated an alert score >= 5 for cluster memory usage",
							},
						},
					},
				},
			},
		},
	}
	// Fetch Prometheus resource to calculate namespace and match labels for rules
	if (r.spec.Services.Metrics.Enabled == nil || *r.spec.Services.Metrics.Enabled) &&
		r.spec.Services.Metrics.PrometheusReference != nil {
		prometheus := &monitoringv1.Prometheus{}
		err := r.client.Get(r.ctx, types.NamespacedName{
			Name:      r.spec.Services.Metrics.PrometheusReference.Name,
			Namespace: r.spec.Services.Metrics.PrometheusReference.Namespace,
		}, prometheus)
		if err != nil {
			return prometheusRule, deploymentState(r.spec.Services.Metrics.Enabled), err
		}
		if prometheus.Spec.RuleNamespaceSelector == nil {
			prometheusRule.SetNamespace(prometheus.Namespace)
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				if r.opniCluster != nil {
					if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
						return err
					}
					if r.opniCluster.Status.PrometheusRuleNamespace != prometheus.Namespace {
						r.opniCluster.Status.PrometheusRuleNamespace = prometheus.Namespace
						err = r.client.Status().Update(r.ctx, r.opniCluster)
						if err != nil {
							return err
						}
					}
					if r.opniCluster.DeletionTimestamp == nil {
						controllerutil.AddFinalizer(r.opniCluster, prometheusRuleFinalizer)
						return r.client.Update(r.ctx, r.opniCluster)
					}
					return nil
				}
				if r.aiOpniCluster != nil {
					if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.aiOpniCluster), r.aiOpniCluster); err != nil {
						return err
					}
					if r.aiOpniCluster.Status.PrometheusRuleNamespace != prometheus.Namespace {
						r.aiOpniCluster.Status.PrometheusRuleNamespace = prometheus.Namespace
						err = r.client.Status().Update(r.ctx, r.aiOpniCluster)
						if err != nil {
							return err
						}
					}
					if r.aiOpniCluster.DeletionTimestamp == nil {
						controllerutil.AddFinalizer(r.aiOpniCluster, prometheusRuleFinalizer)
						return r.client.Update(r.ctx, r.aiOpniCluster)
					}
					return nil
				}
				return errors.New("no opnicluster object to update")
			})
			if err != nil {
				return prometheusRule, deploymentState(r.spec.Services.Metrics.Enabled), err
			}
		} else {
			r.setOwner(prometheusRule)
		}
		if prometheus.Spec.RuleSelector != nil {
			labelSelector, err := metav1.LabelSelectorAsMap(prometheus.Spec.RuleSelector)
			if err != nil {
				return prometheusRule, deploymentState(r.spec.Services.Metrics.Enabled), err
			}
			for k, v := range labelSelector {
				prometheusRule.Labels[k] = v
			}
		}
		return prometheusRule, deploymentState(r.spec.Services.Metrics.Enabled), nil
	}
	return prometheusRule, reconciler.StateAbsent, nil
}

func (r *Reconciler) opensearchUpdateDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(v1beta2.OpensearchUpdateService)
	// Update deployment with additional requirements here
	return deployment, deploymentState(r.spec.Services.OpensearchUpdate.Enabled), nil
}

func (r *Reconciler) generateSHAID() string {
	hash := sha1.New()
	hash.Write([]byte(r.instanceName + r.instanceNamespace))
	sum := hash.Sum(nil)
	return fmt.Sprintf("%x", sum[:3])
}

func insertHyperparametersVolume(deployment *appsv1.Deployment, modelName string) {
	volume := corev1.Volume{
		Name: "hyperparameters",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("opni-%s-hyperparameters", modelName),
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
			MountPath: "/etc/opni/hyperparameters.json",
			SubPath:   "hyperparameters.json",
			ReadOnly:  true,
		})
		deployment.Spec.Template.Spec.Containers[i] = container
	}
}

func (r *Reconciler) externalOpensearchConfig() (retResources []resources.Resource, retErr error) {
	retErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if r.opniCluster != nil {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
				return err
			}
			r.opniCluster.Status.Auth.OpensearchAuthSecretKeyRef = &corev1.SecretKeySelector{
				Key: "password",
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-admin-password", r.opensearchCluster.Name),
				},
			}
			return r.client.Status().Update(r.ctx, r.opniCluster)
		}
		if r.aiOpniCluster != nil {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.aiOpniCluster), r.aiOpniCluster); err != nil {
				return err
			}
			r.aiOpniCluster.Status.Auth.OpensearchAuthSecretKeyRef = &corev1.SecretKeySelector{
				Key: "password",
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-admin-password", r.opensearchCluster.Name),
				},
			}
			return r.client.Status().Update(r.ctx, r.aiOpniCluster)
		}
		return errors.New("no opnicluster object to update")
	})

	return
}
