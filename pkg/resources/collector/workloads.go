package collector

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/rancher/opni/pkg/resources"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	receiversKey       = "receivers.yaml"
	mainKey            = "config.yaml"
	aggregatorKey      = "aggregator.yaml"
	collectorImageRepo = "ghcr.io/rancher-sandbox"
	collectorImage     = "opni-otel-collector"
	collectorVersion   = "v0.1.1-0.74.0"
	otlpGRPCPort       = int32(4317)
	rke2AgentLogDir    = "/var/lib/rancher/rke2/agent/logs/"
	machineID          = "/etc/machine-id"
)

var (
	directoryOrCreate = corev1.HostPathDirectoryOrCreate
)

func (r *Reconciler) receiverConfig() (retData []byte, retReceivers []string, retErr error) {
	if r.collector.Spec.LoggingConfig != nil {
		retData = append(retData, []byte(templateLogAgentK8sReceiver)...)
		retReceivers = append(retReceivers, logReceiverK8s)

		receiver, data, err := r.generateDistributionReceiver()
		if err != nil {
			retErr = err
			return
		}
		retData = append(retData, data...)
		if len(receiver) > 0 {
			retReceivers = append(retReceivers, receiver...)
		}
	}
	return
}

func (r *Reconciler) agentConfigMapName() string {
	return fmt.Sprintf("%s-agent-config", r.collector.Name)
}

func (r *Reconciler) aggregatorConfigMapName() string {
	return fmt.Sprintf("%s-aggregator-config", r.collector.Name)
}

func (r *Reconciler) mainConfig(receivers []string) ([]byte, error) {
	config := AgentConfig{
		Instance: r.collector.Name,
		Logs: LoggingConfig{
			Enabled:   r.collector.Spec.LoggingConfig != nil,
			Receivers: receivers,
		},
	}

	var buffer bytes.Buffer
	err := templateMainConfig.Execute(&buffer, config)
	if err != nil {
		return buffer.Bytes(), err
	}

	return buffer.Bytes(), nil
}

func (r *Reconciler) agentConfigMap() (resources.Resource, string) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.agentConfigMapName(),
			Namespace: r.collector.Spec.SystemNamespace,
			Labels: map[string]string{
				resources.PartOfLabel: "opni",
			},
		},
		Data: map[string]string{},
	}

	receiverData, receivers, err := r.receiverConfig()
	if err != nil {
		return resources.Error(cm, err), ""
	}
	cm.Data[receiversKey] = string(receiverData)

	mainData, err := r.mainConfig(receivers)
	if err != nil {
		return resources.Error(cm, err), ""
	}
	cm.Data[mainKey] = string(mainData)

	combinedData := append(receiverData, mainData...)
	hash := sha256.New()
	hash.Write(combinedData)
	configHash := hex.EncodeToString(hash.Sum(nil))

	if r.collector.Spec.IsEmpty() {
		return resources.Absent(cm), ""
	}

	ctrl.SetControllerReference(r.collector, cm, r.client.Scheme())
	return resources.Present(cm), configHash
}

func (r *Reconciler) aggregatorConfigMap() (resources.Resource, string) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.aggregatorConfigMapName(),
			Namespace: r.collector.Spec.SystemNamespace,
			Labels: map[string]string{
				resources.PartOfLabel: "opni",
			},
		},
		Data: map[string]string{},
	}

	var buffer bytes.Buffer
	err := templateAggregatorConfig.Execute(&buffer, AggregatorConfig{
		LogsEnabled:   r.collector.Spec.LoggingConfig != nil,
		AgentEndpoint: r.collector.Spec.AgentEndpoint,
	})
	if err != nil {
		return resources.Error(nil, err), ""
	}
	config := buffer.Bytes()
	cm.Data[aggregatorKey] = string(config)
	hash := sha256.New()
	hash.Write(config)
	configHash := hex.EncodeToString(hash.Sum(nil))

	if r.collector.Spec.IsEmpty() {
		return resources.Absent(cm), ""
	}

	ctrl.SetControllerReference(r.collector, cm, r.client.Scheme())
	return resources.Present(cm), configHash
}

func (r *Reconciler) daemonSet(configHash string) resources.Resource {
	imageSpec := r.imageSpec()
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "collector-config",
			MountPath: "/etc/otel",
		},
	}
	volumes := []corev1.Volume{
		{
			Name: "collector-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.agentConfigMapName(),
					},
				},
			},
		},
	}
	if r.collector.Spec.LoggingConfig != nil {
		hostVolumeMounts, hostVolumes, err := r.hostLoggingVolumes()
		if err != nil {
			return resources.Error(nil, err)
		}
		volumeMounts = append(volumeMounts, hostVolumeMounts...)
		volumes = append(volumes, hostVolumes...)
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-collector-agent", r.collector.Name),
			Namespace: r.collector.Spec.SystemNamespace,
			Labels: map[string]string{
				resources.AppNameLabel:  "collector-agent",
				resources.PartOfLabel:   "opni",
				resources.InstanceLabel: r.collector.Name,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					resources.AppNameLabel:  "collector-agent",
					resources.PartOfLabel:   "opni",
					resources.InstanceLabel: r.collector.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						resources.AppNameLabel:  "collector-agent",
						resources.PartOfLabel:   "opni",
						resources.InstanceLabel: r.collector.Name,
					},
					Annotations: map[string]string{
						resources.OpniConfigHash: configHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "otel-collector",
							Command: []string{
								"/otelcol-custom",
								fmt.Sprintf("--config=/etc/otel/%s", mainKey),
							},
							Image:           *imageSpec.Image,
							ImagePullPolicy: imageSpec.GetImagePullPolicy(),
							SecurityContext: &corev1.SecurityContext{
								SELinuxOptions: &corev1.SELinuxOptions{
									Type: "rke_logreader_t",
								},
								RunAsUser: lo.ToPtr[int64](0),
							},
							Env: []corev1.EnvVar{
								{
									Name: "NODE_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "spec.nodeName",
										},
									},
								},
								{
									Name: "HOST_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.hostIP",
										},
									},
								},
							},
							VolumeMounts: volumeMounts,
						},
					},
					ImagePullSecrets: imageSpec.ImagePullSecrets,
					Volumes:          volumes,
					Tolerations: []corev1.Toleration{
						{
							Key:    "node-role.kubernetes.io/controlplane",
							Value:  "true",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "node-role.kubernetes.io/control-plane",
							Value:  "true",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "node-role.kubernetes.io/etcd",
							Value:  "true",
							Effect: corev1.TaintEffectNoExecute,
						},
					},
					ServiceAccountName: "opni-agent",
				},
			},
		},
	}

	if r.collector.Spec.IsEmpty() {
		return resources.Absent(ds)
	}

	ctrl.SetControllerReference(r.collector, ds, r.client.Scheme())
	return resources.Present(ds)
}

func (r *Reconciler) deployment(configHash string) resources.Resource {
	imageSpec := r.imageSpec()
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-collector-aggregator", r.collector.Name),
			Namespace: r.collector.Spec.SystemNamespace,
			Labels: map[string]string{
				resources.AppNameLabel:  "collector-aggregator",
				resources.PartOfLabel:   "opni",
				resources.InstanceLabel: r.collector.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					resources.AppNameLabel:  "collector-aggregator",
					resources.PartOfLabel:   "opni",
					resources.InstanceLabel: r.collector.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						resources.AppNameLabel:  "collector-aggregator",
						resources.PartOfLabel:   "opni",
						resources.InstanceLabel: r.collector.Name,
					},
					Annotations: map[string]string{
						resources.OpniConfigHash: configHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "otel-collector",
							Command: []string{
								"/otelcol-custom",
								fmt.Sprintf("--config=/etc/otel/%s", aggregatorKey),
							},
							Image:           *imageSpec.Image,
							ImagePullPolicy: imageSpec.GetImagePullPolicy(),
							Env: []corev1.EnvVar{
								{
									Name: "NODE_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "spec.nodeName",
										},
									},
								},
								{
									Name: "HOST_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.hostIP",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "collector-config",
									MountPath: "/etc/otel",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "otlp-grpc",
									ContainerPort: otlpGRPCPort,
								},
							},
						},
					},
					ImagePullSecrets: imageSpec.ImagePullSecrets,
					Volumes: []corev1.Volume{
						{
							Name: "collector-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: r.aggregatorConfigMapName(),
									},
								},
							},
						},
					},
					ServiceAccountName: "opni-agent",
				},
			},
		},
	}

	if r.collector.Spec.IsEmpty() {
		return resources.Absent(deploy)
	}

	ctrl.SetControllerReference(r.collector, deploy, r.client.Scheme())
	return resources.Present(deploy)
}

func (r *Reconciler) service() resources.Resource {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-otel-aggregator", r.collector.Name),
			Namespace: r.collector.Spec.SystemNamespace,
			Labels: map[string]string{
				resources.AppNameLabel:  "collector-aggregator",
				resources.PartOfLabel:   "opni",
				resources.InstanceLabel: r.collector.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "otlp-grpc",
					Protocol:   corev1.ProtocolTCP,
					Port:       otlpGRPCPort,
					TargetPort: intstr.FromInt(int(otlpGRPCPort)),
				},
			},
			Selector: map[string]string{
				resources.AppNameLabel:  "collector-aggregator",
				resources.PartOfLabel:   "opni",
				resources.InstanceLabel: r.collector.Name,
			},
		},
	}
	ctrl.SetControllerReference(r.collector, svc, r.client.Scheme())

	return resources.Present(svc)
}

func (r *Reconciler) imageSpec() opnimeta.ImageSpec {
	return opnimeta.ImageResolver{
		Version:       collectorVersion,
		ImageName:     collectorImage,
		DefaultRepo:   collectorImageRepo,
		ImageOverride: &r.collector.Spec.ImageSpec,
	}.Resolve()
}
