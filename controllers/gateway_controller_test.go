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
	})
})
