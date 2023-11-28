package controllers_test

import (
	"context"
	"strings"
	"time"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apicorev1 "github.com/rancher/opni/apis/core/v1"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/pkg/util/k8sutil"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
)

var _ = Describe("Core Gateway Controller", Ordered, Label("controller", "slow"), func() {
	When("creating a new empty Gateway object", func() {
		var gw *apicorev1.Gateway
		It("should initialize the active configuration using SSA", func() {
			ns := makeTestNamespace()
			gw = &apicorev1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: ns,
				},
				Spec: apicorev1.GatewaySpec{},
			}
			gw.SetNamespace(ns)
			Expect(k8sClient.Create(context.Background(), gw)).To(Succeed())
			Eventually(Object(gw)).Should(Exist())

			// patch the certificates and create secrets to simulate cert-manager behavior
			Expect(k8sClient.Create(context.Background(), &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-gateway-ca-keys",
					Namespace: gw.Namespace,
				},
				Data: map[string][]byte{
					"ca.crt": []byte("test-ca"),
					"ca.key": []byte("test-ca-key"),
				},
			})).To(Succeed())
			Expect(k8sClient.Create(context.Background(), &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-gateway-serving-cert",
					Namespace: gw.Namespace,
				},
				Data: map[string][]byte{
					"tls.crt": []byte("test-cert"),
					"tls.key": []byte("test-cert-key"),
				},
			})).To(Succeed())

			cert1 := &cmv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-gateway-ca",
					Namespace: gw.Namespace,
				},
			}
			Eventually(Object(cert1)).Should(Exist())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cert1), cert1)).To(Succeed())

			cert2 := &cmv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-gateway-serving-cert",
					Namespace: gw.Namespace,
				},
			}
			Eventually(Object(cert2)).Should(Exist())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cert2), cert2)).To(Succeed())

			ready := cmv1.CertificateStatus{
				Conditions: []cmv1.CertificateCondition{
					{
						ObservedGeneration: 1,
						Reason:             "Ready",
						Type:               cmv1.CertificateConditionReady,
						LastTransitionTime: lo.ToPtr(metav1.NewTime(time.Now().Round(time.Second))),
						Status:             cmmeta.ConditionTrue,
						Message:            "Certificate is up to date and has not expired",
					},
				},
			}
			cert1.Status = ready
			cert2.Status = ready
			Expect(k8sClient.Status().Update(context.Background(), cert1)).To(Succeed())
			Expect(k8sClient.Status().Update(context.Background(), cert2)).To(Succeed())

			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cert1), cert1)).To(Succeed())
			Expect(cert1.Status).To(Equal(ready))

			defaults := &configv1.GatewayConfigSpec{}
			flagutil.LoadDefaults(defaults)
			Eventually(Object(gw)).Should(WithTransform(func(gw *apicorev1.Gateway) *configv1.GatewayConfigSpec {
				return gw.Spec.Config
			}, testutil.ProtoEqual(
				&configv1.GatewayConfigSpec{
					Certs: &configv1.CertsSpec{
						CaCertData:      lo.ToPtr("test-ca"),
						ServingCertData: lo.ToPtr("test-cert"),
						ServingKey:      lo.ToPtr("/run/opni/certs/tls.key"),
					},
					Server: &configv1.ServerSpec{
						HttpListenAddress: defaults.Server.HttpListenAddress,
						GrpcListenAddress: defaults.Server.GrpcListenAddress,
						AdvertiseAddress:  lo.ToPtr("${POD_IP}:9090"),
					},
					Management: &configv1.ManagementServerSpec{
						HttpListenAddress: defaults.Management.HttpListenAddress,
						GrpcListenAddress: defaults.Management.GrpcListenAddress,
						AdvertiseAddress:  lo.ToPtr("${POD_IP}:11090"),
					},
					Relay: &configv1.RelayServerSpec{
						GrpcListenAddress: defaults.Relay.GrpcListenAddress,
						AdvertiseAddress:  lo.ToPtr("${POD_IP}:11190"),
					},
					Health: defaults.Health,
					Dashboard: &configv1.DashboardServerSpec{
						HttpListenAddress: defaults.Dashboard.HttpListenAddress,
						AdvertiseAddress:  lo.ToPtr("${POD_IP}:12080"),
					},
					Storage: &configv1.StorageSpec{
						Backend: configv1.StorageBackend_Etcd.Enum(),
						Etcd: &configv1.EtcdSpec{
							Endpoints: []string{"etcd:2379"},
							Certs: &configv1.MTLSSpec{
								ServerCA:   lo.ToPtr("/run/etcd/certs/server/ca.crt"),
								ClientCA:   lo.ToPtr("/run/etcd/certs/client/ca.crt"),
								ClientCert: lo.ToPtr("/run/etcd/certs/client/tls.crt"),
								ClientKey:  lo.ToPtr("/run/etcd/certs/client/tls.key"),
							},
						},
					},
					Plugins: defaults.Plugins,
					Keyring: &configv1.KeyringSpec{
						RuntimeKeyDirs: []string{"/run/opni/keyring"},
					},
					Upgrades:     defaults.Upgrades,
					RateLimiting: defaults.RateLimiting,
					// this defaults to nil via flags, but must be populated in ssa
					Auth: &configv1.AuthSpec{},
				},
			)))

			Eventually(Object(gw)).Should(WithTransform(func(gw *apicorev1.Gateway) []string {
				set := k8sutil.DecodeManagedFieldsEntry(gw.GetManagedFields()[0])
				var paths []string
				set.Iterate(func(p fieldpath.Path) {
					s := p.String()
					if strings.HasPrefix(s, ".spec.config.") {
						paths = append(paths, strings.TrimPrefix(s, ".spec.config."))
					}
				})
				mask := &fieldmaskpb.FieldMask{Paths: paths}
				mask.Normalize()
				return mask.GetPaths()
			}, ConsistOf([]string{
				"auth",
				"certs",
				"dashboard",
				"health",
				"keyring",
				"management",
				"plugins",
				"rateLimiting",
				"relay",
				"server",
				"storage",
				"upgrades",
			})))
		})
	})
})

var _ = XDescribe("Core Gateway Controller", Ordered, Label("controller", "slow"), func() {
	testCases := []struct {
		name    string
		objects []client.Object
	}{
		{
			name: "etcd storage backend",
			objects: []client.Object{
				&apicorev1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Spec: apicorev1.GatewaySpec{
						Image: &opnimeta.ImageSpec{
							Image: lo.ToPtr("rancher/opni:latest"),
						},
						Config: &configv1.GatewayConfigSpec{
							Storage: &configv1.StorageSpec{
								Backend: configv1.StorageBackend_Etcd.Enum(),
							},
						},
					},
				},
			},
		},
		{
			name: "jetstream storage backend",
			objects: []client.Object{
				&corev1beta1.NatsCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Spec: corev1beta1.NatsSpec{
						AuthMethod: corev1beta1.NatsAuthNkey,
						JetStream: corev1beta1.JetStreamSpec{
							Enabled: lo.ToPtr(true),
						},
					},
				},
				&apicorev1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Spec: apicorev1.GatewaySpec{
						Image: &opnimeta.ImageSpec{
							Image: lo.ToPtr("rancher/opni:latest"),
						},
						NatsRef: corev1.LocalObjectReference{
							Name: "test",
						},
						Config: &configv1.GatewayConfigSpec{
							Storage: &configv1.StorageSpec{
								Backend:   configv1.StorageBackend_JetStream.Enum(),
								JetStream: &configv1.JetStreamSpec{},
							},
						},
					},
				},
			},
		},
		{
			name: "openid auth",
			objects: []client.Object{
				&apicorev1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Spec: apicorev1.GatewaySpec{
						Image: &opnimeta.ImageSpec{
							Image: lo.ToPtr("rancher/opni:latest"),
						},
						Config: &configv1.GatewayConfigSpec{
							Auth: &configv1.AuthSpec{
								Backend: configv1.AuthSpec_OpenID,
								Openid: &configv1.OpenIDAuthSpec{
									Issuer:       lo.ToPtr("https://test-issuer/"),
									ClientId:     lo.ToPtr("test-client-id"),
									ClientSecret: lo.ToPtr("test-client-secret"),
									Scopes:       []string{"openid", "profile", "email"},
								},
							},
							Storage: &configv1.StorageSpec{
								Backend: configv1.StorageBackend_Etcd.Enum(),
								Etcd:    &configv1.EtcdSpec{},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		Context(tc.name, func() {
			var gw *apicorev1.Gateway
			Specify("creating resources", func() {
				ns := makeTestNamespace()
				for _, obj := range tc.objects {
					if g, ok := obj.(*apicorev1.Gateway); ok {
						gw = g
					}
					obj.SetNamespace(ns)
					Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
					Eventually(Object(obj)).Should(Exist())
				}
			})

			It("should create the gateway statefulset", func() {
				Eventually(Object(&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-gateway",
						Namespace: gw.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(gw),
					HaveMatchingContainer(And(
						HaveImage("rancher/opni:latest"),
						HavePorts(
							"http",
							"metrics",
							"management-grpc",
							"management-http",
							"management-web",
							"noauth",
						),
						HaveVolumeMounts(
							"config",
							"certs",
							"cortex-client-certs",
							"cortex-server-cacert",
							"alerting-storage",
							"local-agent-key",
						),
					)),
					HaveMatchingVolume(And(
						HaveName("config"),
						HaveVolumeSource("ConfigMap"),
					)),
					HaveMatchingVolume(And(
						HaveName("certs"),
						HaveVolumeSource("Secret"),
					)),
					HaveMatchingVolume(And(
						HaveName("cortex-client-certs"),
						HaveVolumeSource("Secret"),
					)),
					HaveMatchingVolume(And(
						HaveName("cortex-server-cacert"),
						HaveVolumeSource("Secret"),
					)),
					HaveMatchingVolume(And(
						HaveName("local-agent-key"),
						HaveVolumeSource("Secret"),
					)),
				))
			})

			It("should create the gateway services", func() {
				Eventually(Object(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni",
						Namespace: gw.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(gw),
					HavePorts(
						"grpc",
					),
					HaveType(corev1.ServiceTypeLoadBalancer),
				))
				Eventually(Object(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-internal",
						Namespace: gw.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(gw),
					HavePorts(
						"http",
						"management-grpc",
						"management-http",
					),
					Not(HavePorts(
						"management-web",
					)),
					HaveType(corev1.ServiceTypeClusterIP),
				))
				Eventually(Object(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-admin-dashboard",
						Namespace: gw.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(gw),
					HavePorts(
						"web",
					),
					HaveType(corev1.ServiceTypeClusterIP),
				))
			})
			It("should create the gateway configmap", func() {
				Eventually(Object(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-gateway",
						Namespace: gw.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(gw),
					HaveData("config.yaml", nil),
				))
			})

			It("should create gateway rbac", func() {
				Eventually(Object(&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni",
						Namespace: gw.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(gw),
				))
				Eventually(Object(&rbacv1.Role{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-crd",
						Namespace: gw.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(gw),
				))
				Eventually(Object(&rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-crd",
						Namespace: gw.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(gw),
				))
			})

			It("should create the gateway servicemonitor", func() {
				Eventually(Object(&monitoringv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-gateway",
						Namespace: gw.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(gw),
				))
			})
		})
	}
})
