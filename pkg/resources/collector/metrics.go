package collector

import (
	"bytes"
	"fmt"

	monitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/otel"
	"github.com/rancher/opni/pkg/resources"
	promdiscover "github.com/rancher/opni/pkg/resources/collector/discovery"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	metricsDaemonReceiver = `
{{ template "metrics-node-receivers" .}}
`
	walDir        = "/etc/otel/prometheus/wal"
	tlsSecretName = "opni-otel-tls-assets"
)

func (r *Reconciler) metricsNodeReceiverConfig(
	config otel.NodeConfig,
) (retData []byte, retErr error) {
	if r.collector.Spec.MetricsConfig != nil {
		var b bytes.Buffer
		t, err := r.tmpl.Parse(metricsDaemonReceiver)
		if err != nil {
			return b.Bytes(), err
		}
		if err := t.Execute(&b, config); err != nil {
			return b.Bytes(), err
		}
		retData = append(retData, b.Bytes()...)
	}
	return
}

func (r *Reconciler) getMetricsConfig() (config *otel.MetricsConfig) {
	config = &otel.MetricsConfig{
		Enabled:             false,
		ListenPort:          8888,
		RemoteWriteEndpoint: "",
		WALDir:              walDir,
	}

	if r.collector.Spec.MetricsConfig != nil {
		var metricsConfig monitoringv1beta1.CollectorConfig
		err := r.client.Get(r.ctx, types.NamespacedName{
			Name:      otel.MetricsCrdName,
			Namespace: r.collector.Namespace,
		}, &metricsConfig)
		if err != nil {
			r.lg.Error("error", logger.Err(err))
			return
		}
		config.Enabled = true
		config.RemoteWriteEndpoint = metricsConfig.Spec.RemoteWriteEndpoint
		config.Spec = &metricsConfig.Spec.OtelSpec
		r.PrometheusDiscovery = lo.ToPtr(
			promdiscover.NewPrometheusDiscovery(
				r.lg.With("component", "prometheus-discovery"),
				r.client,
				r.collector.Spec.SystemNamespace,
				metricsConfig.Spec.PrometheusDiscovery,
			),
		)
	}
	if !config.Enabled {
		r.PrometheusDiscovery = nil
	}
	return
}

func (r *Reconciler) withPrometheusCrdDiscovery(
	config *otel.MetricsConfig) (
	*otel.MetricsConfig,
	[]promdiscover.SecretResolutionConfig,
) {
	if r.PrometheusDiscovery == nil {
		return config, []promdiscover.SecretResolutionConfig{}
	}
	discStr, secrets, err := r.discoveredScrapeCfg(config)
	if err != nil {
		r.lg.Warn("failed to discover prometheus targets : %s", err)
	}
	config.DiscoveredScrapeCfg = discStr
	return config, secrets
}

func (r *Reconciler) discoveredScrapeCfg(
	_ *otel.MetricsConfig, // TODO : eventually this config will drive selector config for SD
) (
	retCfg string,
	secrets []promdiscover.SecretResolutionConfig,
	retErr error,
) {
	cfgs, secrets, err := r.PrometheusDiscovery.YieldScrapeConfigs()
	if err != nil || len(cfgs) == 0 {
		return "", []promdiscover.SecretResolutionConfig{}, err
	}
	return otel.PromCfgToString(cfgs), secrets, nil
}

func (r *Reconciler) metricsTlsAssets(sec []promdiscover.SecretResolutionConfig) resources.Resource {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tlsSecretName,
			Namespace: r.collector.Spec.SystemNamespace,
			Labels: map[string]string{
				resources.PartOfLabel: "opni",
			},
		},
		Data: map[string][]byte{},
	}
	for _, s := range sec {
		secret.Data[s.Key()] = s.GetData()
	}
	ctrl.SetControllerReference(r.collector, secret, r.client.Scheme())
	return resources.PresentIff(r.collector.Spec.MetricsConfig != nil, secret)
}

func (r *Reconciler) hostMetricsVolumes() (
	retVolumeMounts []corev1.VolumeMount,
	retVolumes []corev1.Volume,
	retErr error,
) {
	retVolumes = append(retVolumes, corev1.Volume{
		Name: "root",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/",
			},
		},
	})
	retVolumeMounts = append(retVolumeMounts, corev1.VolumeMount{
		Name:             "root",
		ReadOnly:         true,
		MountPath:        "/hostfs",
		MountPropagation: lo.ToPtr(corev1.MountPropagationHostToContainer),
	})

	return
}

func (r *Reconciler) aggregatorMetricVolumes() (retVolumeMounts []corev1.VolumeMount, retVolumes []corev1.Volume) {
	retVolumeMounts = append(retVolumeMounts,
		corev1.VolumeMount{
			Name:      "prom-rw",
			MountPath: walDir,
		},
		corev1.VolumeMount{
			Name:      fmt.Sprintf("%s-volume", tlsSecretName),
			MountPath: fmt.Sprintf(promdiscover.TlsAssetMountPath),
		},
	)

	retVolumes = append(retVolumes,
		corev1.Volume{
			Name: "prom-rw",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					// 120Mb
					SizeLimit: resource.NewQuantity(120*1024*1024, resource.DecimalSI),
				},
			},
		},
		corev1.Volume{
			Name: fmt.Sprintf("%s-volume", tlsSecretName),
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: tlsSecretName,
				},
			},
		},
	)
	return
}
