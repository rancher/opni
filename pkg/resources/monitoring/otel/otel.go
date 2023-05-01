package otel

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	otlpPort            = 4317
	collectorImageRepo  = "ghcr.io/rancher-sandbox"
	collectorImage      = "opni-otel-collector"
	collectorVersion    = "v0.1.1-0.74.0"
	forwarderConfigName = "metrics-forwarder-config"
	forwarderConfigKey  = "config.yaml"
)

// OTLP input
// Cortex remote write outptut
var gatewayForwarderConfig = template.Must(template.New("gatewayForwarderConfig").Parse(`
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: ":{{ .SinkPort }}"
	prometheus/self:
	  config:
		scrape_configs:
		- job_name: 'otel-collector'
			scrape_interval: 15s
			static_configs:
			- targets: ['127.0.0.1:{{ .TelemetryPort }}']
exporters:
  prometheusremotewrite/cortex:
    endpoint: "{{ .CortexRemoteWriteAddress }}"
	tls_config: 
	  ca_file: /etc/otel/run/cortex/certs/client/ca.crt
	  cert_file: /etc/otel/run/cortex/certs/client/tls.crt
	  key_file: /etc/otel/run/cortex/certs/client/tls.key
    resource_to_telemetry_conversion:
      enabled: true
service:
  telemetry:
    metrics:
      address: "127.0.0.1:{{ .TelemetryPort }}"
  pipelines:
    metrics:
      receivers:
	    - otlp
		- prometheus/self
      exporters: [prometheusremotewrite/cortex]
`))

type forwarderConfig struct {
	TelemetryPort            int
	SinkPort                 int
	CortexRemoteWriteAddress string
	CaFile                   string
	CertFile                 string
	KeyFile                  string
}

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx    context.Context
	client client.Client
	logger *zap.SugaredLogger
	mc     *corev1beta1.MonitoringCluster
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	mc *corev1beta1.MonitoringCluster,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			reconciler.WithEnableRecreateWorkload(),
			reconciler.WithRecreateErrorMessageCondition(reconciler.MatchImmutableErrorMessages),
			reconciler.WithLog(log.FromContext(ctx)),
			reconciler.WithScheme(client.Scheme()),
		),
		ctx:    ctx,
		client: client,
		mc:     mc,
		logger: logger.New().Named("controller").Named("metrics-forwarder"),
	}

}

func (r *Reconciler) Reconcile() (*reconcile.Result, error) {
	allResources := []resources.Resource{}
	allResources = append(allResources, r.configMap())
	allResources = append(allResources, r.deployment()...)
	allResources = append(allResources, r.service())
	if op := resources.ReconcileAll(r, allResources); op.ShouldRequeue() {
		return op.ResultPtr()
	}
	return nil, nil
}

func (r *Reconciler) imageSpec() opnimeta.ImageSpec {
	return opnimeta.ImageResolver{
		Version:     collectorVersion,
		ImageName:   collectorImage,
		DefaultRepo: collectorImageRepo,
	}.Resolve()
}

func (r *Reconciler) configMapData() ([]byte, error) {
	//FIXME: do not hardcode distributor address
	config := forwarderConfig{
		TelemetryPort:            8888,
		SinkPort:                 otlpPort,
		CortexRemoteWriteAddress: "https://cortex-distributor:8080/distributor/api/v1/push",
		CaFile:                   "/run/cortex/certs/client/ca.crt",
		CertFile:                 "/run/cortex/certs/client/tls.crt",
		KeyFile:                  "/run/cortex/certs/client/tls.key",
	}
	var b bytes.Buffer
	if err := gatewayForwarderConfig.Execute(&b, config); err != nil {
		return b.Bytes(), err
	}
	return b.Bytes(), nil
}

func (r *Reconciler) configMap() resources.Resource {
	data, err := r.configMapData()
	if err != nil {
		panic(err)
	}

	cfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      forwarderConfigName,
			Namespace: r.mc.Namespace,
		},
		Data: map[string]string{
			forwarderConfigKey: string(data),
		},
	}
	ctrl.SetControllerReference(r.mc, cfgMap, r.client.Scheme())
	return resources.PresentIff(r.mc.Spec.OTEL.Enabled, cfgMap)
}

func (r *Reconciler) deployment() []resources.Resource {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "metrics-forwarder-config",
			MountPath: "/etc/otel",
		},
		{
			Name:      "client-certs",
			MountPath: "/run/cortex/certs/client",
			ReadOnly:  true,
		},
	}
	volumeMounts = append(volumeMounts, r.mc.Spec.OTEL.ExtraVolumeMounts...)
	volumes := []corev1.Volume{
		{
			Name: "metrics-forwarder-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: forwarderConfigName,
					},
				},
			},
		},
		{
			Name: "client-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "cortex-serving-cert-keys",
					Items: []corev1.KeyToPath{
						{
							Key:  "tls.crt",
							Path: "tls.crt",
						},
						{
							Key:  "tls.key",
							Path: "tls.key",
						},
						{
							Key:  "ca.crt",
							Path: "ca.crt",
						},
					},
					DefaultMode: lo.ToPtr[int32](0644),
				},
			},
		},
	}
	volumes = append(volumes, r.mc.Spec.OTEL.ExtraVolumes...)

	imageSpec := r.imageSpec()
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "metrics-forwarder",
			Namespace: r.mc.Namespace,
			Labels: map[string]string{
				resources.AppNameLabel: "metrics-forwarder",
				resources.PartOfLabel:  "opni",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					resources.AppNameLabel: "metrics-forwarder",
					resources.PartOfLabel:  "opni",
				},
			},
			Replicas: lo.ToPtr(lo.FromPtrOr(r.mc.Spec.OTEL.Replicas, 1)),
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						resources.AppNameLabel: "metrics-forwarder",
						resources.PartOfLabel:  "opni",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "metrics-forwarder",
							Image:           *imageSpec.Image,
							ImagePullPolicy: imageSpec.GetImagePullPolicy(),
							Command: []string{
								"otelcol-custom",
								fmt.Sprintf("--config=/etc/otel/%s", forwarderConfigKey),
							},
							Env: append(r.mc.Spec.OTEL.ExtraEnvVars, []corev1.EnvVar{
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
							}...),
							VolumeMounts: volumeMounts,
							Ports: []corev1.ContainerPort{
								{
									Name:          "otlp-grpc",
									ContainerPort: otlpPort,
								},
							},
						},
					},
					ImagePullSecrets: imageSpec.ImagePullSecrets,
					Volumes:          volumes,
				},
			},
		},
	}

	ctrl.SetControllerReference(r.mc, deploy, r.client.Scheme())
	return []resources.Resource{
		resources.PresentIff(r.mc.Spec.OTEL.Enabled, deploy),
	}
}

func (r *Reconciler) service() resources.Resource {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "metrics-forwarder",
			Namespace: r.mc.Namespace,
			Labels: map[string]string{
				resources.AppNameLabel: "metrics-forwarder",
				resources.PartOfLabel:  "opni",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				resources.AppNameLabel: "metrics-forwarder",
				resources.PartOfLabel:  "opni",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "otlp-grpc",
					Protocol:   corev1.ProtocolTCP,
					Port:       otlpPort,
					TargetPort: intstr.FromInt(int(otlpPort)),
				},
			},
		},
	}
	ctrl.SetControllerReference(r.mc, svc, r.client.Scheme())
	return resources.PresentIff(r.mc.Spec.OTEL.Enabled, svc)
}
