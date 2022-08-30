package controllers

import (
	"context"
	"fmt"
	"os"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/apis/v1beta2"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Monitoring Controller", Ordered, Label("controller", "slow"), func() {
	gateway := &types.NamespacedName{}
	testImage := "monitoring-controller-test:latest"
	volumeMounts := []any{"data", "config", "runtime-config", "client-certs", "server-certs", "etcd-client-certs", "etcd-server-cacert"}
	BeforeAll(func() {
		os.Setenv("OPNI_DEBUG_MANAGER_IMAGE", testImage)
		DeferCleanup(os.Unsetenv, "OPNI_DEBUG_MANAGER_IMAGE")
	})

	BeforeEach(func() {
		gw := &v1beta2.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: makeTestNamespace(),
			},
			Spec: v1beta2.GatewaySpec{
				Auth: v1beta2.AuthSpec{
					Provider: cfgv1beta1.AuthProviderNoAuth,
					Noauth:   &noauth.ServerConfig{},
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), gw)).To(Succeed())
		Eventually(Object(gw)).Should(Exist())
		*gateway = types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}
	})

	Context("cortex configuration", func() {
		When("using the AllInOne mode", func() {
			It("should create a single workload with all cortex components", func() {
				aio := &v1beta2.MonitoringCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cortex",
						Namespace: gateway.Namespace,
					},
					Spec: v1beta2.MonitoringClusterSpec{
						Gateway: corev1.LocalObjectReference{
							Name: gateway.Name,
						},
						Cortex: v1beta2.CortexSpec{
							Enabled:        true,
							DeploymentMode: v1beta2.DeploymentModeAllInOne,
							ExtraEnvVars: []corev1.EnvVar{
								{
									Name:  "FOO",
									Value: "BAR",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), aio)).To(Succeed())
				Eventually(Object(aio)).Should(Exist())
				Eventually(Object(&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cortex-all",
						Namespace: aio.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(aio),
					HaveReplicaCount(1),
					HaveMatchingContainer(And(
						HaveName("all"),
						HavePorts("http-metrics", "gossip", "grpc"),
						HaveEnv("FOO", "BAR"),
						HaveVolumeMounts(volumeMounts...),
					)),
				))

				// only 1 statefulset should be created
				Expect(List(&appsv1.StatefulSetList{}, &client.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set{
						"app.kubernetes.io/name": "cortex",
					}),
				})()).To(HaveLen(1))

				// no deployments should be created
				Expect(List(&appsv1.DeploymentList{}, &client.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set{
						"app.kubernetes.io/name": "cortex",
					}),
				})()).To(HaveLen(0))
			})
		})
		When("using the HighlyAvailable mode", func() {
			It("should deploy separate workloads for cortex components", func() {
				aio := &v1beta2.MonitoringCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cortex",
						Namespace: gateway.Namespace,
					},
					Spec: v1beta2.MonitoringClusterSpec{
						Gateway: corev1.LocalObjectReference{
							Name: gateway.Name,
						},
						Cortex: v1beta2.CortexSpec{
							Enabled:        true,
							DeploymentMode: v1beta2.DeploymentModeHighlyAvailable,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), aio)).To(Succeed())
				Eventually(Object(aio)).Should(Exist())

				statefulsets := []string{"compactor", "store-gateway", "ingester", "querier"}
				deployments := []string{"distributor", "query-frontend", "purger", "ruler"}

				for _, target := range statefulsets {
					Eventually(Object(&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("cortex-%s", target),
							Namespace: aio.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(aio),
						HaveMatchingContainer(And(
							HaveName(target),
							HaveImage(testImage),
							HaveVolumeMounts(volumeMounts...),
						)),
					))
				}
				for _, target := range deployments {
					Eventually(Object(&appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("cortex-%s", target),
							Namespace: aio.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(aio),
						HaveMatchingContainer(And(
							HaveName(target),
							HaveImage(testImage),
							HaveVolumeMounts(volumeMounts...),
						)),
					))
				}

				// ensure cortex-all is not created
				Consistently(Object(&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cortex-all",
						Namespace: aio.Namespace,
					},
				})).ShouldNot(Exist())
			})
		})
	})
})
