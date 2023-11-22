package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/logger"

	"emperror.dev/errors"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/auth/openid"
	cfgmeta "github.com/rancher/opni/pkg/config/meta"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	promcommon "github.com/prometheus/common/config"
	yamlv3 "gopkg.in/yaml.v3"
)

func (r *Reconciler) configMap() (resources.Resource, string, error) {
	gatewayConf := &cfgv1beta1.GatewayConfig{
		TypeMeta: cfgmeta.TypeMeta{
			Kind:       "GatewayConfig",
			APIVersion: "v1beta1",
		},
		Spec: cfgv1beta1.GatewayConfigSpec{
			Plugins: cfgv1beta1.PluginsSpec{
				Dir: "/var/lib/opni/plugins",
				Binary: cfgv1beta1.BinaryPluginsSpec{
					Cache: cfgv1beta1.CacheSpec{
						PatchEngine: r.gw.Spec.PatchEngine,
						Backend:     cfgv1beta1.CacheBackendFilesystem,
						Filesystem: cfgv1beta1.FilesystemCacheSpec{
							Dir: "/var/lib/opni/plugin-cache",
						},
					},
				},
			},
			Hostname: r.gw.Spec.Hostname,
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
			AuthProvider: string(r.gw.Spec.Auth.Provider),
			Certs: cfgv1beta1.CertsSpec{
				CACert:      lo.ToPtr("/run/opni/certs/ca.crt"),
				ServingCert: lo.ToPtr("/run/opni/certs/tls.crt"),
				ServingKey:  lo.ToPtr("/run/opni/certs/tls.key"),
			},
			Storage: cfgv1beta1.StorageSpec{
				Type: r.gw.Spec.StorageType,
			},
			Alerting: cfgv1beta1.AlertingSpec{
				Certs: cfgv1beta1.MTLSSpec{
					ServerCA:   "/run/alerting/certs/server/ca.crt",
					ClientCA:   "/run/alerting/certs/client/ca.crt",
					ClientCert: "/run/alerting/certs/client/tls.crt",
					ClientKey:  "/run/alerting/certs/client/tls.key",
				},
				Namespace:             r.gw.Namespace,
				WorkerNodeService:     shared.AlertmanagerService,
				WorkerPort:            r.gw.Spec.Alerting.WebPort,
				WorkerStatefulSet:     shared.AlertmanagerService + "-internal",
				ControllerNodeService: shared.AlertmanagerService,
				ControllerStatefulSet: shared.AlertmanagerService + "-internal",
				ControllerNodePort:    r.gw.Spec.Alerting.WebPort,
				ControllerClusterPort: r.gw.Spec.Alerting.ClusterPort,
				ConfigMap:             "alertmanager-config",
			},
			Keyring: cfgv1beta1.KeyringSpec{
				EphemeralKeyDirs: []string{
					"/run/opni/keyring",
				},
			},
			GRPCListenAddress:    "0.0.0.0:9090",
			GRPCAdvertiseAddress: "${POD_IP}:9090",
			Management: cfgv1beta1.ManagementSpec{
				GRPCListenAddress:     "tcp://0.0.0.0:11090",
				GRPCAdvertiseAddress:  "tcp://${POD_IP}:11090",
				RelayListenAddress:    "tcp://0.0.0.0:11190",
				RelayAdvertiseAddress: "tcp://${POD_IP}:11190",
				WebListenAddress:      "0.0.0.0:12080",
				WebAdvertiseAddress:   "${POD_IP}:12080",
				HTTPListenAddress:     "0.0.0.0:11080",
			},
			AgentUpgrades: cfgv1beta1.AgentUpgradesSpec{
				Kubernetes: cfgv1beta1.KubernetesAgentUpgradeSpec{
					ImageResolver: cfgv1beta1.ImageResolverKubernetes,
				},
			},
		},
	}
	gatewayConf.Spec.SetDefaults()

	if r.gw.Status.StorageType != "" && r.gw.Status.StorageType != r.gw.Spec.StorageType {
		return nil, "", fmt.Errorf("storage type cannot be changed once set")
	} else if r.gw.Status.StorageType == "" {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.gw), r.gw)
			if err != nil {
				return err
			}
			r.gw.Status.StorageType = r.gw.Spec.StorageType
			return r.client.Status().Update(r.ctx, r.gw)
		})
		if err != nil {
			return nil, "", err
		}
	}

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
	case cfgv1beta1.StorageTypeJetStream:
		nats := &opnicorev1beta1.NatsCluster{}
		if err := r.client.Get(r.ctx, types.NamespacedName{
			Namespace: r.gw.Namespace,
			Name:      r.gw.Spec.NatsRef.Name,
		}, nats); err != nil {
			return nil, "", fmt.Errorf("failed to look up nats cluster, cannot configure jetstream storage: %w", err)
		}
		gatewayConf.Spec.Storage.JetStream = &cfgv1beta1.JetStreamStorageSpec{
			Endpoint:     natsutil.BuildK8sServiceUrl(nats.Name, nats.Namespace),
			NkeySeedPath: filepath.Join(natsutil.NkeyDir, natsutil.NkeySeedFilename),
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
		options, err := util.DecodeStruct[map[string]any](r.gw.Spec.Auth.Openid.OpenidConfig)
		if err != nil {
			return nil, "", errors.WrapIf(err, "failed to decode openid auth provider options")
		}
		apSpec.Options = *options

	case cfgv1beta1.AuthProviderNoAuth:
		apSpec.Type = cfgv1beta1.AuthProviderNoAuth
		issuer := fmt.Sprintf("http://%s:4000/oauth2", r.gw.Spec.Hostname)
		r.gw.Spec.Auth.Noauth = &noauth.ServerConfig{
			Issuer:       issuer,
			ClientID:     "grafana",
			ClientSecret: "noauth",
			RedirectURI: func() string {
				if r.gw.Spec.Auth.Noauth != nil {
					return fmt.Sprintf("https://%s/login/generic_oauth", r.gw.Spec.Auth.Noauth.GrafanaHostname)
				}
				return fmt.Sprintf("https://grafana.%s/login/generic_oauth", r.gw.Spec.Hostname)
			}(),
			ManagementAPIEndpoint: "opni-internal:11090",
			Port:                  4000,
			OpenID: openid.OpenidConfig{
				Discovery: &openid.DiscoverySpec{
					Issuer: issuer,
				},
			},
		}
		options, err := util.DecodeStruct[map[string]any](r.gw.Spec.Auth.Noauth)
		if err != nil {
			return nil, "", errors.WrapIf(err, "failed to decode noauth auth provider options")
		}
		apSpec.Options = *options
	default:
		return nil, "", errors.Errorf("unsupported auth provider: %s", t)
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
		return nil, "", errors.WrapIf(err, "failed to marshal gateway config")
	}
	authProviderData, err := yaml.Marshal(authProvider)
	if err != nil {
		return nil, "", errors.WrapIf(err, "failed to marshal auth provider")
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
	digest := sha256.Sum256([]byte(cm.Data["config.yaml"]))

	ctrl.SetControllerReference(r.gw, cm, r.client.Scheme())
	return resources.Present(cm), hex.EncodeToString(digest[:]), nil
}

func (r *Reconciler) amtoolConfigMap() resources.Resource {
	mgmtHost := "127.0.0.1:8080"
	alertmanagerURL := url.URL{
		Scheme: "https",
		Host:   mgmtHost,
		Path:   "/plugin_alerting/alertmanager",
	}

	amToolConfig := map[string]string{
		"alertmanager.url": alertmanagerURL.String(),
		"http.config.file": "/etc/amtool/http.yml",
	}

	amToolConfigBytes, err := yamlv3.Marshal(amToolConfig)
	if err != nil {
		r.lg.Error("failed to marshal amtool config", logger.Err(err))
		amToolConfigBytes = []byte{}
	}

	httpConfig := promcommon.HTTPClientConfig{
		TLSConfig: promcommon.TLSConfig{
			CAFile:   "/run/opni/certs/ca.crt",
			CertFile: "/run/opni/certs/tls.crt",
			KeyFile:  "/run/opni/certs/tls.key",
		},
		FollowRedirects: true,
	}
	httpBytes, err := yamlv3.Marshal(httpConfig)
	if err != nil {
		r.lg.Error("failed to marshal http config", logger.Err(err))
		httpBytes = []byte{}
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "amtool-config",
			Namespace: r.gw.Namespace,
		},
		Data: map[string]string{
			"config.yml": string(amToolConfigBytes),
			"http.yml":   string(httpBytes),
		},
	}
	return resources.Present(cm)
}
