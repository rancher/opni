package controllers_test

import (
	"context"
	"fmt"
	"os"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/samber/lo"
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
		ns := makeTestNamespace()
		nc := &corev1beta1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      "test",
			},
			Spec: corev1beta1.NatsSpec{
				AuthMethod: corev1beta1.NatsAuthNkey,
				JetStream: corev1beta1.JetStreamSpec{
					Enabled: lo.ToPtr(true),
				},
			},
		}
		gw := &corev1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: ns,
			},
			Spec: corev1beta1.GatewaySpec{
				Auth: corev1beta1.AuthSpec{
					Provider: cfgv1beta1.AuthProviderNoAuth,
					Noauth:   &noauth.ServerConfig{},
				},
				StorageType: cfgv1beta1.StorageTypeEtcd,
				NatsRef: corev1.LocalObjectReference{
					Name: "test",
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), nc)).To(Succeed())
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
				aio := &corev1beta1.MonitoringCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cortex",
						Namespace: gateway.Namespace,
					},
					Spec: corev1beta1.MonitoringClusterSpec{
						Gateway: corev1.LocalObjectReference{
							Name: gateway.Name,
						},
						Cortex: corev1beta1.CortexSpec{
							Enabled: lo.ToPtr(true),
							CortexWorkloads: &cortexops.CortexWorkloadsConfig{
								Targets: map[string]*cortexops.CortexWorkloadSpec{
									"all": {},
								},
							},
						},
						Grafana: corev1beta1.GrafanaSpec{
							GrafanaConfig: &cortexops.GrafanaConfig{
								Enabled: true,
								Version: "10.0.0",
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

				// grafana should be deployed
				Eventually(Object(&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana",
						Namespace: aio.Namespace,
					},
				})).Should(ExistAnd(
					HaveReplicaCount(1),
					HaveMatchingContainer(And(
						HaveName("grafana"),
						HaveImage("grafana/grafana:10.0.0"),
					)),
				))
			})
		})
		When("using the HighlyAvailable mode", func() {
			It("should deploy separate workloads for cortex components", func() {
				aio := &corev1beta1.MonitoringCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cortex",
						Namespace: gateway.Namespace,
					},
					Spec: corev1beta1.MonitoringClusterSpec{
						Gateway: corev1.LocalObjectReference{
							Name: gateway.Name,
						},
						Cortex: corev1beta1.CortexSpec{
							Enabled: lo.ToPtr(true),
							CortexWorkloads: &cortexops.CortexWorkloadsConfig{
								Targets: map[string]*cortexops.CortexWorkloadSpec{
									"distributor":    {},
									"query-frontend": {},
									"purger":         {},
									"ruler":          {},
									"compactor":      {},
									"store-gateway":  {},
									"ingester":       {},
									"alertmanager":   {},
									"querier":        {},
								},
							},
						},
						Grafana: corev1beta1.GrafanaSpec{
							GrafanaConfig: &cortexops.GrafanaConfig{
								Enabled: true,
							},
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), aio)).To(Succeed())
				Eventually(Object(aio)).Should(Exist())

				statefulsets := []string{"compactor", "store-gateway", "ingester", "querier", "alertmanager"}
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

				// grafana should be deployed
				Eventually(Object(&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana",
						Namespace: aio.Namespace,
					},
				})).Should(ExistAnd(
					HaveReplicaCount(1),
					HaveMatchingContainer(And(
						HaveName("grafana"),
						HaveImage("grafana/grafana:latest"),
					)),
				))
			})
		})
	})
})
