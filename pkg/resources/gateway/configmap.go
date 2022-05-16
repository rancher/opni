package gateway

import (
	"fmt"

	"emperror.dev/errors"
	"github.com/rancher/opni/pkg/auth/openid"
	cfgmeta "github.com/rancher/opni/pkg/config/meta"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/yaml"
)

func (r *Reconciler) configMap() (resources.Resource, error) {
	gatewayConf := &cfgv1beta1.GatewayConfig{
		TypeMeta: cfgmeta.TypeMeta{
			Kind:       "GatewayConfig",
			APIVersion: "v1beta1",
		},
		Spec: cfgv1beta1.GatewayConfigSpec{
			Plugins: cfgv1beta1.PluginsSpec{
				Dirs: append([]string{"/var/lib/opni/plugins"}, r.gw.Spec.PluginSearchDirs...),
			},
			Hostname: r.gw.Spec.Hostname,
			Cortex: cfgv1beta1.CortexSpec{
				Certs: cfgv1beta1.MTLSSpec{
					ServerCA:   "/run/cortex/certs/server/ca.crt",
					ClientCA:   "/run/cortex/certs/client/ca.crt",
					ClientCert: "/run/cortex/certs/client/tls.crt",
					ClientKey:  "/run/cortex/certs/client/tls.key",
				},
			},
			AuthProvider: string(r.gw.Spec.Auth.Provider),
			Certs: cfgv1beta1.CertsSpec{
				CACert:      util.Pointer("/run/opni-monitoring/certs/ca.crt"),
				ServingCert: util.Pointer("/run/opni-monitoring/certs/tls.crt"),
				ServingKey:  util.Pointer("/run/opni-monitoring/certs/tls.key"),
			},
			Storage: cfgv1beta1.StorageSpec{
				Type: r.gw.Spec.StorageType,
			},
		},
	}
	gatewayConf.Spec.SetDefaults()

	switch r.gw.Spec.StorageType {
	case cfgv1beta1.StorageTypeEtcd:
		gatewayConf.Spec.Storage.Etcd = &cfgv1beta1.EtcdStorageSpec{
			Endpoints: []string{"etcd:2379"},
			Certs: &cfgv1beta1.MTLSSpec{
				ServerCA:   "/run/etcd/certs/server/ca.crt",
				ClientCA:   "/run/etcd/certs/client/ca.crt",
				ClientCert: "/run/etcd/certs/client/tls.crt",
				ClientKey:  "/run/etcd/certs/client/tls.key",
			},
		}
	case cfgv1beta1.StorageTypeCRDs:
		gatewayConf.Spec.Storage.CustomResources = &cfgv1beta1.CustomResourcesStorageSpec{
			Namespace: r.gw.Namespace,
		}
	}

	var apSpec cfgv1beta1.AuthProviderSpec
	switch t := cfgv1beta1.AuthProviderType(r.gw.Spec.Auth.Provider); t {
	case cfgv1beta1.AuthProviderOpenID:
		apSpec.Type = cfgv1beta1.AuthProviderOpenID
		if options, err := util.DecodeStruct[map[string]any](r.gw.Spec.Auth.Openid); err != nil {
			return nil, errors.WrapIf(err, "failed to decode openid auth provider options")
		} else {
			apSpec.Options = *options
		}
	case cfgv1beta1.AuthProviderNoAuth:
		apSpec.Type = cfgv1beta1.AuthProviderNoAuth
		issuer := fmt.Sprintf("http://%s:4000/oauth2", r.gw.Spec.Hostname)
		r.gw.Spec.Auth.Noauth = &noauth.ServerConfig{
			Issuer:                issuer,
			ClientID:              "grafana",
			ClientSecret:          "noauth",
			RedirectURI:           fmt.Sprintf("https://%s/generic_oauth", r.gw.Spec.Auth.Noauth.GrafanaHostname),
			ManagementAPIEndpoint: "opni-monitoring-internal:11090",
			Port:                  4000,
			OpenID: openid.OpenidConfig{
				Discovery: &openid.DiscoverySpec{
					Issuer: issuer,
				},
			},
		}
		if options, err := util.DecodeStruct[map[string]any](r.gw.Spec.Auth.Noauth); err != nil {
			return nil, errors.WrapIf(err, "failed to decode noauth auth provider options")
		} else {
			apSpec.Options = *options
		}
	default:
		return nil, errors.Errorf("unsupported auth provider: %s", t)
	}

	authProvider := &cfgv1beta1.AuthProvider{
		TypeMeta: cfgmeta.TypeMeta{
			Kind:       "AuthProvider",
			APIVersion: "v1beta1",
		},
		ObjectMeta: cfgmeta.ObjectMeta{
			Name: string(r.gw.Spec.Auth.Provider),
		},
		Spec: apSpec,
	}

	gatewayConfData, err := yaml.Marshal(gatewayConf)
	if err != nil {
		return nil, errors.WrapIf(err, "failed to marshal gateway config")
	}
	authProviderData, err := yaml.Marshal(authProvider)
	if err != nil {
		return nil, errors.WrapIf(err, "failed to marshal auth provider")
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway",
			Namespace: r.gw.Namespace,
			Labels:    resources.NewGatewayLabels(),
		},
		Data: map[string]string{
			"config.yaml": fmt.Sprintf("%s\n---\n%s", gatewayConfData, authProviderData),
		},
	}
	ctrl.SetControllerReference(r.gw, cm, r.client.Scheme())
	return resources.Present(cm), nil
}
