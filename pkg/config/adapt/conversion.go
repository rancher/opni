package adapt

import (
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/samber/lo"
)

func V1GatewayConfigOf[T *v1beta1.GatewayConfig | *configv1.GatewayConfigSpec](in T) *configv1.GatewayConfigSpec {
	switch in := any(in).(type) {
	case *configv1.GatewayConfigSpec:
		return in
	case *v1beta1.GatewayConfig:
		return &configv1.GatewayConfigSpec{
			Server: &configv1.ServerSpec{
				HttpListenAddress: &in.Spec.HTTPListenAddress,
				GrpcListenAddress: &in.Spec.GRPCListenAddress,
			},
			Management: &configv1.ManagementServerSpec{
				HttpListenAddress: lo.ToPtr(in.Spec.Management.GetHTTPListenAddress()),
				GrpcListenAddress: lo.ToPtr(in.Spec.Management.GetGRPCListenAddress()),
			},
			Relay: &configv1.RelayServerSpec{
				GrpcListenAddress: &in.Spec.Management.RelayListenAddress,
				AdvertiseAddress:  &in.Spec.Management.RelayAdvertiseAddress,
			},
			Health: &configv1.HealthServerSpec{
				HttpListenAddress: &in.Spec.HTTPListenAddress,
			},
			Dashboard: &configv1.DashboardServerSpec{
				HttpListenAddress: &in.Spec.Management.WebListenAddress,
				Hostname:          &in.Spec.Hostname,
				AdvertiseAddress:  &in.Spec.Management.WebAdvertiseAddress,
				TrustedProxies:    in.Spec.TrustedProxies,
			},
			Storage: &configv1.StorageSpec{
				Backend: func() *configv1.StorageBackend {
					switch in.Spec.Storage.Type {
					case v1beta1.StorageTypeEtcd:
						fallthrough
					default:
						return configv1.StorageBackend_Etcd.Enum()
					case v1beta1.StorageTypeJetStream:
						return configv1.StorageBackend_JetStream.Enum()
					}
				}(),
				Etcd: func() *configv1.EtcdSpec {
					if in.Spec.Storage.Etcd == nil {
						return nil
					}
					return &configv1.EtcdSpec{
						Endpoints: in.Spec.Storage.Etcd.Endpoints,
						Certs: func() *configv1.MTLSSpec {
							if in.Spec.Storage.Etcd.Certs == nil {
								return nil
							}
							return &configv1.MTLSSpec{
								ServerCA:   &in.Spec.Storage.Etcd.Certs.ServerCA,
								ClientCA:   &in.Spec.Storage.Etcd.Certs.ClientCA,
								ClientCert: &in.Spec.Storage.Etcd.Certs.ClientCert,
								ClientKey:  &in.Spec.Storage.Etcd.Certs.ClientKey,
							}
						}(),
					}
				}(),
				JetStream: func() *configv1.JetStreamSpec {
					if in.Spec.Storage.JetStream == nil {
						return nil
					}
					return &configv1.JetStreamSpec{
						Endpoint:     &in.Spec.Storage.JetStream.Endpoint,
						NkeySeedPath: &in.Spec.Storage.JetStream.NkeySeedPath,
					}
				}(),
			},
			Certs: &configv1.CertsSpec{
				CaCert:          in.Spec.Certs.CACert,
				CaCertData:      lo.Ternary(len(in.Spec.Certs.CACertData) > 0, lo.ToPtr(string(in.Spec.Certs.CACertData)), nil),
				ServingCert:     in.Spec.Certs.ServingCert,
				ServingCertData: lo.Ternary(len(in.Spec.Certs.ServingCertData) > 0, lo.ToPtr(string(in.Spec.Certs.ServingCertData)), nil),
				ServingKey:      in.Spec.Certs.ServingKey,
				ServingKeyData:  lo.Ternary(len(in.Spec.Certs.ServingKeyData) > 0, lo.ToPtr(string(in.Spec.Certs.ServingKeyData)), nil),
			},
			Plugins: &configv1.PluginsSpec{
				Dir: &in.Spec.Plugins.Dir,
				Cache: &configv1.CacheSpec{
					Backend: configv1.CacheBackend_Filesystem.Enum(),
					Filesystem: &configv1.FilesystemCacheSpec{
						Dir: &in.Spec.Plugins.Binary.Cache.Filesystem.Dir,
					},
				},
			},
			Keyring: &configv1.KeyringSpec{
				RuntimeKeyDirs: in.Spec.Keyring.EphemeralKeyDirs,
			},
			Upgrades: &configv1.UpgradesSpec{
				Agents: &configv1.AgentUpgradesSpec{
					Driver: func() *configv1.AgentUpgradesSpec_Driver {
						switch in.Spec.AgentUpgrades.Kubernetes.ImageResolver {
						case v1beta1.ImageResolverNoop:
							fallthrough
						default:
							return configv1.AgentUpgradesSpec_Noop.Enum()
						case v1beta1.ImageResolverKubernetes:
							return configv1.AgentUpgradesSpec_Kubernetes.Enum()
						}
					}(),
					Kubernetes: &configv1.KubernetesAgentUpgradeSpec{
						ImageResolver: func() *configv1.KubernetesAgentUpgradeSpec_ImageResolver {
							switch in.Spec.AgentUpgrades.Kubernetes.ImageResolver {
							case v1beta1.ImageResolverNoop:
								fallthrough
							default:
								return configv1.KubernetesAgentUpgradeSpec_Noop.Enum()
							case v1beta1.ImageResolverKubernetes:
								return configv1.KubernetesAgentUpgradeSpec_Kubernetes.Enum()
							}
						}(),
					},
				},
				Plugins: &configv1.PluginUpgradesSpec{
					Driver: configv1.PluginUpgradesSpec_Binary.Enum(),
					Binary: &configv1.BinaryPluginUpgradeSpec{
						PatchEngine: func() *configv1.PatchEngine {
							switch in.Spec.Plugins.Binary.Cache.PatchEngine {
							case v1beta1.PatchEngineBsdiff:
								return configv1.PatchEngine_Bsdiff.Enum()
							case v1beta1.PatchEngineZstd:
								fallthrough
							default:
								return configv1.PatchEngine_Zstd.Enum()
							}
						}(),
					},
				},
			},
			RateLimiting: func() *configv1.RateLimitingSpec {
				if in.Spec.RateLimit == nil {
					return nil
				}
				return &configv1.RateLimitingSpec{
					Rate: &in.Spec.RateLimit.Rate,
					Burst: func() *int32 {
						burst := int32(in.Spec.RateLimit.Burst)
						return &burst
					}(),
				}
			}(),
		}
	}
	panic("unreachable")
}
