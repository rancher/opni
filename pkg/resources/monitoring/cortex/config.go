package cortex

import (
	"fmt"

	"github.com/cortexproject/cortex/pkg/storage/bucket/filesystem"
	"github.com/go-kit/log"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex/configutil"
	"github.com/samber/lo"
	"github.com/weaveworks/common/server"
	"go.uber.org/zap"

	"github.com/cortexproject/cortex/pkg/util/tls"
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) config() ([]resources.Resource, error) {
	if r.mc.Spec.Cortex.Enabled == nil || !*r.mc.Spec.Cortex.Enabled {
		return []resources.Resource{
			resources.Absent(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cortex",
					Namespace: r.mc.Namespace,
					Labels:    cortexAppLabel,
				},
			}),
			resources.Absent(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cortex-runtime-config",
					Namespace: r.mc.Namespace,
					Labels:    cortexAppLabel,
				},
			}),
		}, nil
	}

	if r.mc.Spec.Cortex.CortexConfig == nil {
		r.mc.Spec.Cortex.CortexConfig = &cortexops.CortexApplicationConfig{}
	}

	tlsCortexClientConfig := tls.ClientConfig{
		CAPath:     "/run/cortex/certs/client/ca.crt",
		CertPath:   "/run/cortex/certs/client/tls.crt",
		KeyPath:    "/run/cortex/certs/client/tls.key",
		ServerName: "cortex-server",
	}

	tlsGatewayClientConfig := tls.ClientConfig{
		CAPath:   "/run/gateway/certs/client/ca.crt",
		CertPath: "/run/gateway/certs/client/tls.crt",
		KeyPath:  "/run/gateway/certs/client/tls.key",
	}

	tlsServerConfig := server.TLSConfig{
		TLSCertPath: "/run/cortex/certs/server/tls.crt",
		TLSKeyPath:  "/run/cortex/certs/server/tls.key",
		ClientCAs:   "/run/cortex/certs/client/ca.crt",
		ClientAuth:  "RequireAndVerifyClientCert",
	}

	conf, rtConf, err := configutil.CortexAPISpecToCortexConfig(r.mc.Spec.Cortex.CortexConfig,
		configutil.MergeOverrideLists(
			[]configutil.CortexConfigOverrider{
				configutil.NewOverrider(func(t *filesystem.Config) bool {
					t.Directory = "/data"
					return true
				}),
			},
			configutil.NewHostOverrides(configutil.StandardOverridesShape{
				HttpListenAddress:      "0.0.0.0",
				HttpListenPort:         8080,
				GrpcListenAddress:      "0.0.0.0",
				GrpcListenPort:         9095,
				StorageDir:             "/data",
				RuntimeConfig:          "/etc/cortex-runtime-config/runtime_config.yaml",
				TLSServerConfig:        configutil.TLSServerConfigShape(tlsServerConfig),
				TLSGatewayClientConfig: configutil.TLSClientConfigShape(tlsGatewayClientConfig),
				TLSCortexClientConfig:  configutil.TLSClientConfigShape(tlsCortexClientConfig),
			}),
			configutil.NewImplementationSpecificOverrides(configutil.ImplementationSpecificOverridesShape{
				QueryFrontendAddress: "cortex-query-frontend-headless:9095",
				MemberlistJoinAddrs:  []string{"cortex-memberlist"},
				AlertmanagerURL:      fmt.Sprintf("https://opni-internal.%s.svc:8080/plugin_alerting/alertmanager", r.mc.Namespace),
			}),
		)...,
	)
	if err != nil {
		return nil, err
	}
	if err := conf.Validate(log.NewNopLogger()); err != nil {
		r.logger.With(
			zap.Error(err),
		).Warn("Cortex config failed validation (ignoring)")
	}
	confBytes, err := configutil.MarshalCortexConfig(conf)
	if err != nil {
		r.logger.With(
			zap.Error(err),
		).Error("Failed to marshal cortex config (cannot continue)")
		return nil, err
	}
	rtConfBytes, err := configutil.MarshalRuntimeConfig(rtConf)
	if err != nil {
		r.logger.With(
			zap.Error(err),
		).Error("Failed to marshal cortex runtime config (cannot continue)")
		return nil, err
	}
	configSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex",
			Namespace: r.mc.Namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string][]byte{
			"cortex.yaml": confBytes,
		},
	}
	runtimeConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-runtime-config",
			Namespace: r.mc.Namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string]string{
			"runtime_config.yaml": string(rtConfBytes),
		},
	}

	ctrl.SetControllerReference(r.mc, configSecret, r.client.Scheme())
	ctrl.SetControllerReference(r.mc, runtimeConfigMap, r.client.Scheme())
	return []resources.Resource{
		resources.Present(configSecret),
		resources.Present(runtimeConfigMap),
	}, nil
}

func (r *Reconciler) alertmanagerFallbackConfig() resources.Resource {
	cfgStr := `global: {}
templates: []
route:
	receiver: default
receivers:
	- name: default
inhibit_rules: []
mute_time_intervals: []`
	dConfig, err := shared.DefaultAlertManagerConfig(
		"http://127.0.0.1:3000",
	)
	if err == nil {
		cfgStr = dConfig.String()
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alertmanager-fallback-config",
			Namespace: r.mc.Namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string]string{
			"fallback.yaml": cfgStr,
		},
	}
	ctrl.SetControllerReference(r.mc, cm, r.client.Scheme())
	return resources.PresentIff(lo.FromPtr(r.mc.Spec.Cortex.Enabled), cm)
}
