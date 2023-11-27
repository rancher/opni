package opnicluster

import (
	"crypto/sha1"
	"fmt"
	"net/url"

	"emperror.dev/errors"
	"github.com/Opster/opensearch-k8s-operator/opensearch-operator/pkg/helpers"
	opensearchk8s "github.com/Opster/opensearch-k8s-operator/opensearch-operator/pkg/reconcilers/k8s"
	"github.com/cisco-open/operator-tools/pkg/reconciler"
	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) opniServices() ([]resources.Resource, error) {
	return []resources.Resource{
		r.nulogHyperparameters,
		r.inferenceDeployment,
		r.drainDeployment,
		r.payloadReceiverDeployment,
		r.payloadReceiverService,
		r.preprocessingDeployment,
		r.gpuCtrlDeployment,
		r.metricsDeployment,
		r.metricsService,
		r.metricsServiceMonitor,
		r.metricsPrometheusRule,
		r.opensearchUpdateDeployment,
		r.trainingControllerDeployment,
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
	models := map[string]corev1.LocalObjectReference{}
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
	model, err := r.findPretrainedModel(modelRef)
	if err != nil {
		return nil, err
	}

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
			r.serviceLabels(aiv1beta1.InferenceService),
			r.pretrainedModelLabels(model.Name),
		)
		imageSpec := r.serviceImageSpec(aiv1beta1.InferenceService)
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
				Namespace: r.opniCluster.Namespace,
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
						Tolerations:      r.serviceTolerations(aiv1beta1.InferenceService),
						NodeSelector:     r.serviceNodeSelector(aiv1beta1.InferenceService),
					},
				},
			},
		}
		ctrl.SetControllerReference(r.opniCluster, deployment, r.client.Scheme())
		insertHyperparametersVolume(deployment, model.Name)

		return deployment, reconciler.StatePresent, nil
	}, nil
}

func (r *Reconciler) workloadDrain() (resourceList []resources.Resource, retError error) {
	resourceList = []resources.Resource{}
	workloadDeployment, err := r.workloadDrainDeployment()
	if err != nil {
		return nil, err
	}
	resourceList = append(resourceList, workloadDeployment)
	return resourceList, nil
}

func (r *Reconciler) workloadDrainDeployment() (resources.Resource, error) {
	return func() (runtime.Object, reconciler.DesiredState, error) {
		labels := resources.CombineLabels(
			r.serviceLabels(aiv1beta1.DrainService),
		)
		imageSpec := r.serviceImageSpec(aiv1beta1.DrainService)
		lg, _ := logr.FromContext(r.ctx)
		lg.V(1).Info("generating workload DRAIN deployment")
		envVars, volumeMounts, volumes := r.genericEnvAndVolumes()
		s3EnvVars := r.s3EnvVars()
		envVars = append(envVars, s3EnvVars...)
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "FAIL_KEYWORDS",
				Value: "fail,error,missing,unable",
			})
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "IS_PRETRAINED_SERVICE",
				Value: "false",
			})
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("opni-workload-drain"),
				Namespace: r.opniCluster.Namespace,
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
						Volumes: volumes,
						Containers: []corev1.Container{
							{
								Name:            "workload-drain-service",
								Image:           imageSpec.GetImage(),
								ImagePullPolicy: imageSpec.GetImagePullPolicy(),
								VolumeMounts:    volumeMounts,
								Env:             envVars,
							},
						},
						Tolerations:  r.serviceTolerations(aiv1beta1.DrainService),
						NodeSelector: r.serviceNodeSelector(aiv1beta1.DrainService),
					},
				},
			},
		}
		ctrl.SetControllerReference(r.opniCluster, deployment, r.client.Scheme())
		var state = reconciler.StateAbsent
		if lo.FromPtrOr(r.opniCluster.Spec.Services.Drain.Workload.Enabled, false) && lo.FromPtrOr(r.opniCluster.Spec.Services.Drain.Enabled, true) {
			state = reconciler.StatePresent
		}
		return deployment, state, nil
	}, nil
}

func maybeImagePullSecrets(model *aiv1beta1.PretrainedModel) []corev1.LocalObjectReference {
	if model.Spec.Container != nil {
		return model.Spec.Container.ImagePullSecrets
	}
	return nil
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
	imageSpec := r.serviceImageSpec(aiv1beta1.InferenceService)
	envVars, volumeMounts, _ := r.genericEnvAndVolumes()
	envVars = append(envVars, r.s3EnvVars()...)
	envVars = append(envVars, []corev1.EnvVar{
		{
			Name:  "MODEL_THRESHOLD",
			Value: "0.7",
		},
		{
			Name:  "MIN_LOG_TOKENS",
			Value: "1",
		},
		{
			Name:  "SERVICE_TYPE",
			Value: "gpu",
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
	if r.opniCluster.Spec.S3.NulogS3Bucket != "" {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "S3_BUCKET",
				Value: r.opniCluster.Spec.S3.NulogS3Bucket,
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
) (*aiv1beta1.PretrainedModel, error) {
	model := &aiv1beta1.PretrainedModel{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      modelRef.Name,
		Namespace: r.opniCluster.Namespace,
	}, model)
	if err != nil {
		return nil, err
	}
	return model, nil
}

func (r *Reconciler) genericDeployment(service aiv1beta1.ServiceKind) *appsv1.Deployment {
	labels := r.serviceLabels(service)
	imageSpec := r.serviceImageSpec(service)
	envVars, volumeMounts, volumes := r.genericEnvAndVolumes()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels[resources.AppNameLabel],
			Namespace: r.opniCluster.Namespace,
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

	ctrl.SetControllerReference(r.opniCluster, deployment, r.client.Scheme())
	return deployment
}

func (r *Reconciler) genericEnvAndVolumes() (
	envVars []corev1.EnvVar,
	volumeMounts []corev1.VolumeMount,
	volumes []corev1.Volume,
) {
	newEnvVars, newVolumeMounts, newVolumes := nats.ExternalNatsObjects(
		r.ctx,
		r.client,
		types.NamespacedName{
			Name:      r.opniCluster.Spec.NatsRef.Name,
			Namespace: r.opniCluster.Namespace,
		},
	)
	envVars = append(envVars, newEnvVars...)
	volumes = append(volumes, newVolumes...)
	volumeMounts = append(volumeMounts, newVolumeMounts...)
	envVars = append(envVars, corev1.EnvVar{
		Name: "ES_ENDPOINT",
		Value: fmt.Sprintf(
			"https://%s.%s.svc:9200",
			r.opensearchCluster.Spec.General.ServiceName,
			r.opensearchCluster.Namespace,
		),
	}, corev1.EnvVar{
		Name: "ES_USERNAME",
		Value: func() string {
			opensearchClient := opensearchk8s.NewK8sClient(r.client, r.ctx)
			user, _, _ := helpers.UsernameAndPassword(
				opensearchClient,
				r.opensearchCluster,
			)
			return user
		}(),
	}, corev1.EnvVar{
		Name: "ES_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: r.opniCluster.Status.Auth.OpensearchAuthSecretKeyRef,
		},
	})
	return
}

func (r *Reconciler) s3EnvVars() (envVars []corev1.EnvVar) {
	// lg := logr.FromContext(r.ctx)
	if r.opniCluster.Status.Auth.S3AccessKey != nil &&
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
	return envVars
}

func deploymentState(enabled *bool) reconciler.DesiredState {
	if lo.FromPtrOr(enabled, true) {
		return reconciler.StatePresent
	}
	return reconciler.StateAbsent
}

func deploymentStateDisabled(enabled *bool) reconciler.DesiredState {
	if lo.FromPtrOr(enabled, false) {
		return reconciler.StatePresent
	}
	return reconciler.StateAbsent
}

func (r *Reconciler) nulogHyperparameters() (runtime.Object, reconciler.DesiredState, error) {
	var data map[string]intstr.IntOrString
	if len(r.opniCluster.Spec.NulogHyperparameters) > 0 {
		data = r.opniCluster.Spec.NulogHyperparameters
	} else {
		data = map[string]intstr.IntOrString{
			"modelThreshold": intstr.FromString("0.7"),
			"minLogTokens":   intstr.FromInt(1),
		}
	}
	cm, err := hyperparameters.GenerateHyperparametersConfigMap("nulog", r.opniCluster.Namespace, data)
	if err != nil {
		return nil, nil, err
	}
	cm.Labels["opni-service"] = "nulog"
	ctrl.SetControllerReference(r.opniCluster, &cm, r.client.Scheme())
	return &cm, reconciler.StatePresent, err
}

func (r *Reconciler) inferenceDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(aiv1beta1.InferenceService)
	addCPUInferenceLabel(deployment)
	s3EnvVars := r.s3EnvVars()
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, s3EnvVars...)
	if r.opniCluster.Spec.S3.NulogS3Bucket != "" {
		deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "S3_BUCKET",
				Value: r.opniCluster.Spec.S3.NulogS3Bucket,
			})
	}
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  "SERVICE_TYPE",
		Value: "cpu",
	})

	insertHyperparametersVolume(deployment, "nulog")
	return deployment, deploymentStateDisabled(r.opniCluster.Spec.Services.Inference.Enabled), nil
}

func (r *Reconciler) drainDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(aiv1beta1.DrainService)
	s3EnvVars := r.s3EnvVars()
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, s3EnvVars...)
	if r.opniCluster.Spec.S3.DrainS3Bucket != "" {
		deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "S3_BUCKET",
				Value: r.opniCluster.Spec.S3.DrainS3Bucket,
			})
	}
	deployment.Spec.Replicas = r.opniCluster.Spec.Services.Drain.Replicas
	return deployment, deploymentState(r.opniCluster.Spec.Services.Drain.Enabled), nil
}

func (r *Reconciler) trainingControllerDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(aiv1beta1.TrainingControllerService)
	s3EnvVars := r.s3EnvVars()
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, s3EnvVars...)
	return deployment, deploymentStateDisabled(r.opniCluster.Spec.Services.TrainingController.Enabled), nil
}

func (r *Reconciler) payloadReceiverDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(aiv1beta1.PayloadReceiverService)
	return deployment, deploymentState(r.opniCluster.Spec.Services.PayloadReceiver.Enabled), nil
}

func (r *Reconciler) payloadReceiverService() (runtime.Object, reconciler.DesiredState, error) {
	labels := r.serviceLabels(aiv1beta1.PayloadReceiverService)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels[resources.AppNameLabel],
			Namespace: r.opniCluster.Namespace,
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
	ctrl.SetControllerReference(r.opniCluster, service, r.client.Scheme())
	return service, deploymentState(r.opniCluster.Spec.Services.PayloadReceiver.Enabled), nil
}

func (r *Reconciler) preprocessingDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(aiv1beta1.PreprocessingService)
	deployment.Spec.Replicas = r.opniCluster.Spec.Services.Preprocessing.Replicas
	return deployment, deploymentState(r.opniCluster.Spec.Services.Preprocessing.Enabled), nil
}

func (r *Reconciler) gpuCtrlDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(aiv1beta1.GPUControllerService)
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
	deployment.Spec.Template.Spec.RuntimeClassName = r.opniCluster.Spec.Services.GPUController.RuntimeClass
	deployment.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType
	deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, r.gpuWorkerContainer())
	deployment.Spec.Template.Spec.Containers = deployment.Spec.Template.Spec.Containers[1:]

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
	return deployment, deploymentStateDisabled(r.opniCluster.Spec.Services.GPUController.Enabled), nil
}

func (r *Reconciler) getPrometheusEndpoint() (endpoint string) {
	lg := log.FromContext(r.ctx)
	if r.opniCluster.Spec.Services.Metrics.Enabled != nil && !*r.opniCluster.Spec.Services.Metrics.Enabled {
		return
	}
	if r.opniCluster.Spec.Services.Metrics.PrometheusReference != nil {
		prometheus := &monitoringv1.Prometheus{}
		err := r.client.Get(r.ctx, types.NamespacedName{
			Name:      r.opniCluster.Spec.Services.Metrics.PrometheusReference.Name,
			Namespace: r.opniCluster.Spec.Services.Metrics.PrometheusReference.Namespace,
		}, prometheus)
		if err != nil {
			lg.V(1).Error(err, "unable to fetch prometheus")
		} else {
			endpoint = prometheus.Spec.ExternalURL
		}
	}
	if r.opniCluster.Spec.Services.Metrics.PrometheusEndpoint != "" {
		endpoint = r.opniCluster.Spec.Services.Metrics.PrometheusEndpoint
	}

	return
}

func (r *Reconciler) metricsDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(aiv1beta1.MetricsService)
	prometheusEndpoint := r.getPrometheusEndpoint()
	_, err := url.ParseRequestURI(prometheusEndpoint)
	if err != nil && (r.opniCluster.Spec.Services.Metrics.Enabled == nil || *r.opniCluster.Spec.Services.Metrics.Enabled) {
		return deployment, deploymentState(r.opniCluster.Spec.Services.Metrics.Enabled), errors.New("prometheus endpoint is not a valid URL")
	}
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
		Name:  "PROMETHEUS_ENDPOINT",
		Value: prometheusEndpoint,
	})
	for _, extraVolume := range r.opniCluster.Spec.Services.Metrics.ExtraVolumeMounts {
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
	return deployment, deploymentState(r.opniCluster.Spec.Services.Metrics.Enabled), nil
}

func (r *Reconciler) metricsService() (runtime.Object, reconciler.DesiredState, error) {
	labels := r.serviceLabels(aiv1beta1.MetricsService)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels[resources.AppNameLabel],
			Namespace: r.opniCluster.Namespace,
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
	ctrl.SetControllerReference(r.opniCluster, service, r.client.Scheme())
	return service, deploymentState(r.opniCluster.Spec.Services.Metrics.Enabled), nil
}

func (r *Reconciler) metricsServiceMonitor() (runtime.Object, reconciler.DesiredState, error) {
	labels := r.serviceLabels(aiv1beta1.MetricsService)
	serviceMonitor := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels[resources.AppNameLabel],
			Namespace: r.opniCluster.Namespace,
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
	ctrl.SetControllerReference(r.opniCluster, serviceMonitor, r.client.Scheme())
	if r.opniCluster.Spec.Services.Metrics.PrometheusReference == nil {
		return serviceMonitor, reconciler.StateAbsent, nil
	}
	return serviceMonitor, deploymentState(r.opniCluster.Spec.Services.Metrics.Enabled), nil
}

func (r *Reconciler) metricsPrometheusRule() (runtime.Object, reconciler.DesiredState, error) {
	labels := r.serviceLabels(aiv1beta1.MetricsService)
	prometheusRule := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", labels[resources.AppNameLabel], r.generateSHAID()),
			Namespace: r.opniCluster.Namespace,
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
	if (r.opniCluster.Spec.Services.Metrics.Enabled == nil || *r.opniCluster.Spec.Services.Metrics.Enabled) &&
		r.opniCluster.Spec.Services.Metrics.PrometheusReference != nil {
		prometheus := &monitoringv1.Prometheus{}
		err := r.client.Get(r.ctx, types.NamespacedName{
			Name:      r.opniCluster.Spec.Services.Metrics.PrometheusReference.Name,
			Namespace: r.opniCluster.Spec.Services.Metrics.PrometheusReference.Namespace,
		}, prometheus)
		if err != nil {
			return prometheusRule, deploymentState(r.opniCluster.Spec.Services.Metrics.Enabled), err
		}
		if prometheus.Spec.RuleNamespaceSelector == nil {
			prometheusRule.SetNamespace(prometheus.Namespace)
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
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
			})
			if err != nil {
				return prometheusRule, deploymentState(r.opniCluster.Spec.Services.Metrics.Enabled), err
			}
		} else {
			ctrl.SetControllerReference(r.opniCluster, prometheusRule, r.client.Scheme())
		}
		if prometheus.Spec.RuleSelector != nil {
			labelSelector, err := metav1.LabelSelectorAsMap(prometheus.Spec.RuleSelector)
			if err != nil {
				return prometheusRule, deploymentState(r.opniCluster.Spec.Services.Metrics.Enabled), err
			}
			for k, v := range labelSelector {
				prometheusRule.Labels[k] = v
			}
		}
		return prometheusRule, deploymentState(r.opniCluster.Spec.Services.Metrics.Enabled), nil
	}
	return prometheusRule, reconciler.StateAbsent, nil
}

func (r *Reconciler) opensearchUpdateDeployment() (runtime.Object, reconciler.DesiredState, error) {
	deployment := r.genericDeployment(aiv1beta1.OpensearchUpdateService)
	// Update deployment with additional requirements here
	return deployment, deploymentState(r.opniCluster.Spec.Services.OpensearchUpdate.Enabled), nil
}

func (r *Reconciler) generateSHAID() string {
	hash := sha1.New()
	hash.Write([]byte(r.opniCluster.Name + r.opniCluster.Namespace))
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
	})

	return
}
