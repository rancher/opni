package controllers

import (
	"context"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/apis/v1beta2"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	"github.com/rancher/opni/pkg/util"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Gateway Controller", Ordered, Label("controller", "slow"), func() {
	When("creating a Gateway resource", func() {
		var gw *v1beta2.Gateway
		It("should succeed", func() {
			gw = &v1beta2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: makeTestNamespace(),
				},
				Spec: v1beta2.GatewaySpec{
					Image: &v1beta2.ImageSpec{
						Image: util.Pointer("rancher/opni:latest"),
					},
					Auth: v1beta2.AuthSpec{
						Provider: cfgv1beta1.AuthProviderNoAuth,
						Noauth:   &noauth.ServerConfig{},
					},
					Alerting: &v1beta2.AlertingSpec{
						Enabled:     true,
						ServiceType: corev1.ServiceTypeLoadBalancer,
						WebPort:     9093,
						ApiPort:     9094,
						ConfigName:  "alertmanager-config",
						GatewayVolumeMounts: []opnimeta.ExtraVolumeMount{
							{
								Name:      "alerting-storage",
								MountPath: "/var/logs/alerting",
								ReadOnly:  false,
								VolumeSource: corev1.VolumeSource{
									NFS: &corev1.NFSVolumeSource{
										Server: "localhost",
										Path:   "/var/logs/alerting",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), gw)).To(Succeed())
			Eventually(Object(gw)).Should(Exist())
		})

		It("should create the gateway deployment", func() {
			Eventually(Object(&appsv1.Deployment{
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
			))
		})

		It("should create the gateway services", func() {
			Eventually(Object(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-monitoring",
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
					Name:      "opni-monitoring-internal",
					Namespace: gw.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(gw),
				HavePorts(
					"http",
					"management-grpc",
					"management-http",
					"management-web",
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
					Name:      "opni-monitoring",
					Namespace: gw.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(gw),
			))
			Eventually(Object(&rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-monitoring-crd",
					Namespace: gw.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(gw),
			))
			Eventually(Object(&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-monitoring-crd",
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
		It("should create the alerting Objects", func() {
			Eventually(Object(&appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-alerting-internal",
					Namespace: gw.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(gw),
				HaveMatchingContainer(And(
					HaveImage("bitnami/alertmanager:latest"),
					HavePorts(
						"alert-web-port",
						"alert-api-port",
					),
					HaveVolumeMounts(
						"opni-alertmanager-data",
						"opni-alertmanager-config",
					),
				)),
				HaveMatchingVolume(
					And(
						HaveName("opni-alertmanager-data"),
						HaveVolumeSource("PersistentVolumeClaim"),
					),
				),
				HaveMatchingVolume(
					And(
						HaveName("opni-alertmanager-config"),
						HaveVolumeSource("ConfigMap"),
					),
				),
			))

			Eventually(Object(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-alerting",
					Namespace: gw.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(gw),
				HavePorts(
					"alert-web-port",
					"alert-api-port",
				),
				HaveType(corev1.ServiceTypeLoadBalancer),
			))
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gw.Spec.Alerting.ConfigName,
					Namespace: gw.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(gw),
			))
		})
		When("disabling alerting", func() {
			It("should remove the alerting objects", func() {
				updateObject(gw, func(gw *v1beta2.Gateway) *v1beta2.Gateway {
					gw.Spec.Alerting = nil
					return gw
				})
				Eventually(Object(&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-alerting-internal",
						Namespace: gw.Namespace,
					},
				})).ShouldNot(Exist())

				Eventually(Object(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-alerting",
						Namespace: gw.Namespace,
					},
				})).ShouldNot(Exist())
			})
		})
	})
})
