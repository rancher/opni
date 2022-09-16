package gateway

import (
	"fmt"
	"github.com/rancher/opni/pkg/alerting/shared"

	"emperror.dev/errors"
	"github.com/rancher/opni/pkg/auth/openid"
	cfgmeta "github.com/rancher/opni/pkg/config/meta"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				Dirs: append([]string{"/var/lib/opni/plugins"}, r.spec.PluginSearchDirs...),
			},
			Hostname: r.spec.Hostname,
			Cortex: cfgv1beta1.CortexSpec{
				Management: cfgv1beta1.ClusterManagementSpec{
					ClusterDriver: "opni-manager",
				},
				Certs: cfgv1beta1.MTLSSpec{
					ServerCA:   "/run/cortex/certs/server/ca.crt",
					ClientCA:   "/run/cortex/certs/client/ca.crt",
					ClientCert: "/run/cortex/certs/client/tls.crt",
					ClientKey:  "/run/cortex/certs/client/tls.key",
				},
			},
			AuthProvider: string(r.spec.Auth.Provider),
			Certs: cfgv1beta1.CertsSpec{
				CACert:      lo.ToPtr("/run/opni-monitoring/certs/ca.crt"),
				ServingCert: lo.ToPtr("/run/opni-monitoring/certs/tls.crt"),
				ServingKey:  lo.ToPtr("/run/opni-monitoring/certs/tls.key"),
			},
			Storage: cfgv1beta1.StorageSpec{
				Type: r.spec.StorageType,
			},
			Alerting: func() cfgv1beta1.AlertingSpec {
				if r.spec.Alerting == nil {
					return cfgv1beta1.AlertingSpec{
						Namespace:             r.gw.Namespace,
						WorkerNodeService:     shared.OperatorAlertingClusterNodeServiceName,
						WorkerPort:            9093,
						WorkerStatefulSet:     shared.OperatorAlertingClusterNodeServiceName + "-internal",
						ControllerNodeService: shared.OperatorAlertingControllerServiceName,
						ControllerStatefulSet: shared.OperatorAlertingControllerServiceName + "-internal",
						ControllerNodePort:    9093,
						ControllerClusterPort: 9094,
						ConfigMap:             "alertmanager-config",
						ManagementHookHandler: shared.AlertingCortexHookHandler,
						//ManagementHookHandlerName: shared.AlertingCortexHookHandler,
					}
				}
				return cfgv1beta1.AlertingSpec{
					Namespace:             r.gw.Namespace,
					WorkerNodeService:     shared.OperatorAlertingClusterNodeServiceName,
					WorkerPort:            r.gw.Spec.Alerting.WebPort,
					WorkerStatefulSet:     shared.OperatorAlertingClusterNodeServiceName + "-internal",
					ControllerNodeService: shared.OperatorAlertingControllerServiceName,
					ControllerStatefulSet: shared.OperatorAlertingControllerServiceName + "-internal",
					ControllerNodePort:    r.gw.Spec.Alerting.WebPort,
					ControllerClusterPort: r.gw.Spec.Alerting.ClusterPort,
					ConfigMap:             "alertmanager-config",
				}
			}(),
			Profiling: r.spec.Profiling,
		},
	}
	gatewayConf.Spec.SetDefaults()

	switch r.spec.StorageType {
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
			Namespace: r.namespace,
		}
	}

	var apSpec cfgv1beta1.AuthProviderSpec
	switch t := cfgv1beta1.AuthProviderType(r.spec.Auth.Provider); t {
	case cfgv1beta1.AuthProviderOpenID:
		apSpec.Type = cfgv1beta1.AuthProviderOpenID
		if options, err := util.DecodeStruct[map[string]any](r.spec.Auth.Openid.OpenidConfig); err != nil {
			return nil, errors.WrapIf(err, "failed to decode openid auth provider options")
		} else {
			apSpec.Options = *options
		}
	case cfgv1beta1.AuthProviderNoAuth:
		apSpec.Type = cfgv1beta1.AuthProviderNoAuth
		issuer := fmt.Sprintf("http://%s:4000/oauth2", r.spec.Hostname)
		r.spec.Auth.Noauth = &noauth.ServerConfig{
			Issuer:       issuer,
			ClientID:     "grafana",
			ClientSecret: "noauth",
			RedirectURI: func() string {
				if r.spec.Auth.Noauth != nil {
					return fmt.Sprintf("https://%s/login/generic_oauth", r.spec.Auth.Noauth.GrafanaHostname)
				}
				return fmt.Sprintf("https://grafana.%s/login/generic_oauth", r.spec.Hostname)
			}(),
			ManagementAPIEndpoint: "opni-monitoring-internal:11090",
			Port:                  4000,
			OpenID: openid.OpenidConfig{
				Discovery: &openid.DiscoverySpec{
					Issuer: issuer,
				},
			},
		}
		if options, err := util.DecodeStruct[map[string]any](r.spec.Auth.Noauth); err != nil {
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
			Name: string(r.spec.Auth.Provider),
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
			Namespace: r.namespace,
			Labels:    resources.NewGatewayLabels(),
		},
		Data: map[string]string{
			"config.yaml": fmt.Sprintf("%s\n---\n%s", gatewayConfData, authProviderData),
		},
	}

	r.setOwner(cm)
	return resources.Present(cm), nil
}
