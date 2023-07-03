package cortex

import (
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex/configutil"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/server"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/cortexproject/cortex/pkg/util/tls"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/resources"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) config() (resources.Resource, error) {
	if !r.mc.Spec.Cortex.Enabled {
		return resources.Absent(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cortex",
				Namespace: r.mc.Namespace,
				Labels:    cortexAppLabel,
			},
		}), nil
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
		ClientAuth:  "RequireAndVerifyClientCert",
		ClientCAs:   "/run/cortex/certs/client/ca.crt",
	}

	impl := configutil.ImplementationSpecificConfig{
		HttpListenPort:       8080,
		GrpcListenPort:       9095,
		StorageDir:           "/data",
		RuntimeConfig:        "/etc/cortex-runtime-config/runtime_config.yaml",
		TLSServerConfig:      tlsServerConfig,
		TLSClientConfig:      tlsClientConfig,
		QueryFrontendAddress: "cortex-query-frontend-headless:9095",
	}
	conf, err := configutil.CortexAPISpecToCortexConfig(&r.mc.Spec.Cortex, impl)
	if err != nil {
		return nil, err
	}
	buf, err := configutil.MarshalCortexConfig(conf)
	if err != nil {
		return nil, err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex",
			Namespace: r.mc.Namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string][]byte{
			"cortex.yaml": buf,
		},
	}

	ctrl.SetControllerReference(r.mc, secret, r.client.Scheme())
	return resources.Present(secret), nil
}

func (r *Reconciler) runtimeConfig() (resources.Resource, error) {
	// the cortex runtime config is unexported in pkg/cortex/runtime_config.go
	// the only field we use right now is the tenant limits config, which is
	// TenantLimits map[string]*validation.Limits `yaml:"overrides"`

	overrides := map[string]any{}
	for tenantId, limits := range r.mc.Spec.Cortex.TenantLimits {
		limitsData, err := protojson.Marshal(limits)
		if err != nil {
			return nil, err
		}
		var overrideValue any
		if err := yaml.Unmarshal(limitsData, &overrideValue); err != nil {
			return nil, err
		}
		overrides[tenantId] = overrideValue
	}

	yamlOverrides, err := yaml.Marshal(map[string]any{"overrides": overrides})
	if err != nil {
		return nil, err
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-runtime-config",
			Namespace: r.mc.Namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string]string{
			"runtime_config.yaml": string(yamlOverrides),
		},
	}
	ctrl.SetControllerReference(r.mc, cm, r.client.Scheme())
	return resources.CreatedIff(r.mc.Spec.Cortex.Enabled, cm), nil
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
