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
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Monitoring Controller", Label(test.Integration, test.Slow), func() {
	When("creating a MonitoringCluster resource", func() {
		var mc *v1beta2.MonitoringCluster
		It("should succeed", func() {
			mc = &v1beta2.MonitoringCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: makeTestNamespace(),
				},
				Spec: v1beta2.MonitoringClusterSpec{
					Image: &v1beta2.ImageSpec{
						Image: util.Pointer("rancher/opni:latest"),
					},
					Gateway: v1beta2.GatewaySpec{
						Auth: v1beta2.AuthSpec{
							Provider: cfgv1beta1.AuthProviderNoAuth,
							Noauth:   &noauth.ServerConfig{},
						},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), mc)).To(Succeed())
			Eventually(Object(mc)).Should(Exist())
		})

		It("should create the gateway deployment", func() {
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-gateway",
					Namespace: mc.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(mc),
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
					Namespace: mc.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(mc),
				HavePorts(
					"http",
				),
				HaveType(corev1.ServiceTypeLoadBalancer),
			))
			Eventually(Object(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-monitoring-internal",
					Namespace: mc.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(mc),
				HavePorts(
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
					Namespace: mc.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(mc),
				HaveData("config.yaml", nil),
			))
		})

		It("should create gateway rbac", func() {
			Eventually(Object(&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-monitoring",
					Namespace: mc.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(mc),
			))
			Eventually(Object(&rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-monitoring-crd",
					Namespace: mc.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(mc),
			))
			Eventually(Object(&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-monitoring-crd",
					Namespace: mc.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(mc),
			))
		})

		It("should create the gateway servicemonitor", func() {
			Eventually(Object(&monitoringv1.ServiceMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-gateway",
					Namespace: mc.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(mc),
			))
		})
	})
})
