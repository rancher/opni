package controllers_test

import (
	"context"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apicorev1 "github.com/rancher/opni/apis/core/v1"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Core Gateway Controller", Ordered, Label("controller", "slow"), func() {
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
