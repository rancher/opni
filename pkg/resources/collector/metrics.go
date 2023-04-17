package collector

import (
	"bytes"

	monitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
	"github.com/rancher/opni/pkg/otel"
	promdiscover "github.com/rancher/opni/pkg/resources/collector/discovery"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
)

const (
	metricsDaemonReceiver = `
{{ template "metrics-node-receivers" .}}
`
	walDir = "/var/lib/prometheus/wal"
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
			r.logger.Error(err)
			return
		}
		config.Enabled = true
		config.RemoteWriteEndpoint = metricsConfig.Spec.RemoteWriteEndpoint
		config.Spec = &metricsConfig.Spec.OtelSpec
	}
	return
}

func (r *Reconciler) withPrometheusCrdDiscovery(config *otel.MetricsConfig) *otel.MetricsConfig {
	if !config.Enabled {
		r.PrometheusDiscovery = nil
		return config
	}
	if r.PrometheusDiscovery == nil {
		r.PrometheusDiscovery = lo.ToPtr(
			promdiscover.NewPrometheusDiscovery(
				r.logger.With("component", "prometheus-discovery"),
				r.client,
			))
	}
	discStr, err := r.discoveredScrapeCfg(config)
	if err != nil {
		r.logger.Warn("failed to discover prometheus targets : %s", err)
	}
	config.DiscoveredScrapeCfg = discStr
	return config
}

func (r *Reconciler) discoveredScrapeCfg(_ *otel.MetricsConfig) (retCfg string, retErr error) {
	cfgs, err := r.PrometheusDiscovery.YieldScrapeConfigs()
	if err != nil || len(cfgs) == 0 {
		return "", err
	}
	return otel.PromCfgToString(cfgs), nil
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
	)
	return
}
