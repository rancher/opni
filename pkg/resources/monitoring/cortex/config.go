package cortex

import (
	"fmt"

	"github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex/configutil"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/server"

	"github.com/cortexproject/cortex/pkg/util/tls"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) config() ([]resources.Resource, error) {
	if !r.mc.Spec.Cortex.Enabled {
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

	if r.mc.Spec.Cortex.Storage == nil {
		r.logger.Warn("No cortex storage configured; using volatile EmptyDir storage. It is recommended to configure a storage backend.")
		r.mc.Spec.Cortex.Storage = &storagev1.StorageSpec{
			Backend: storagev1.Filesystem,
			Filesystem: &storagev1.FilesystemStorageSpec{
				Directory: "/data",
			},
		}
	}

	logLevel := logging.Level{}
	level := r.mc.Spec.Cortex.LogLevel
	if level == "" {
		level = "info"
	}
	if err := logLevel.Set(level); err != nil {
		return nil, err
	}
	logFmt := logging.Format{}
	logFmt.Set("json")

	tlsClientConfig := tls.ClientConfig{
		CAPath:     "/run/cortex/certs/client/ca.crt",
		CertPath:   "/run/cortex/certs/client/tls.crt",
		KeyPath:    "/run/cortex/certs/client/tls.key",
		ServerName: "cortex-server",
	}
	tlsServerConfig := server.TLSConfig{
		TLSCertPath: "/run/cortex/certs/server/tls.crt",
		TLSKeyPath:  "/run/cortex/certs/server/tls.key",
		ClientCAs:   "/run/cortex/certs/client/ca.crt",
		ClientAuth:  "RequireAndVerifyClientCert",
	}
	overrideLists := [][]configutil.CortexConfigOverrider{
		configutil.NewStandardOverrides(configutil.StandardOverridesShape{
			HttpListenAddress: "0.0.0.0",
			HttpListenPort:    8080,
			HttpListenNetwork: "tcp4",
			GrpcListenAddress: "0.0.0.0",
			GrpcListenPort:    9095,
			GrpcListenNetwork: "tcp4",
			StorageDir:        "/data",
			RuntimeConfig:     "/etc/cortex-runtime-config/runtime_config.yaml",
			TLSServerConfig:   configutil.TLSServerConfigShape(tlsServerConfig),
			TLSClientConfig:   configutil.TLSClientConfigShape(tlsClientConfig),
		}),
		configutil.NewImplementationSpecificOverrides(configutil.ImplementationSpecificOverridesShape{
			QueryFrontendAddress: "cortex-query-frontend-headless:9095",
			MemberlistJoinAddrs:  []string{"cortex-memberlist"},
			AlertmanagerURL:      fmt.Sprintf("http://%s:9093", shared.OperatorAlertingControllerServiceName),
		}),
	}
	switch r.mc.Spec.Cortex.DeploymentMode {
	case v1beta1.DeploymentModeHighlyAvailable:
		overrideLists = append(overrideLists, configutil.NewHAOverrides())
	}

	conf, rtConf, err := configutil.CortexAPISpecToCortexConfig(&r.mc.Spec.Cortex,
		configutil.MergeOverrideLists(overrideLists...)...,
	)
	if err != nil {
		return nil, err
	}
	confBytes, err := configutil.MarshalCortexConfig(conf)
	if err != nil {
		return nil, err
	}
	rtConfBytes, err := configutil.MarshalRuntimeConfig(rtConf)
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
	return resources.PresentIff(r.mc.Spec.Cortex.Enabled, cm)
}
