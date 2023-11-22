package controllers_test

import (
	"context"
	"os"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apicorev1 "github.com/rancher/opni/apis/core/v1"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/alerting/shared"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/apimachinery/pkg/types"
)

var _ client.Object = (*corev1beta1.AlertingCluster)(nil)

var _ = Describe("Alerting Controller", Ordered, Label("controller", "slow"), func() {
	gateway := &types.NamespacedName{}
	testImage := "alerting-controller-test:latest"
	BeforeAll(func() {
		os.Setenv("OPNI_DEBUG_MANAGER_IMAGE", testImage)
		DeferCleanup(os.Unsetenv, "OPNI_DEBUG_MANAGER_IMAGE")
	})

	BeforeEach(func() {
		gw := &apicorev1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: makeTestNamespace(),
			},
			Spec: apicorev1.GatewaySpec{},
		}
		Expect(k8sClient.Create(context.Background(), gw)).To(Succeed())
		Eventually(Object(gw)).Should(Exist())
		*gateway = types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}
	})

	Context("alerting configuration", func() {
		When("using standalone mode", func() {
			It("should create the alerting resources", func() {
				cl := &corev1beta1.AlertingCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "alerting",
						Namespace: gateway.Namespace,
					},
					Spec: corev1beta1.AlertingClusterSpec{
						Gateway: corev1.LocalObjectReference{
							Name: gateway.Name,
						},
						Alertmanager: corev1beta1.AlertManagerSpec{
							Enable:     true,
							DeployConf: corev1beta1.AlertingDeployConfStandalone,
							ApplicationSpec: corev1beta1.AlertingApplicationSpec{
								ExtraEnvVars: []corev1.EnvVar{
									{
										Name:  "FOO",
										Value: "BAR",
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), cl)).To(Succeed())
				Eventually(Object(cl)).Should(Exist())
				Eventually(Object(&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      shared.AlertmanagerService,
						Namespace: gateway.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(cl),
					HaveReplicaCount(1),
					HaveMatchingContainer(And(
						HaveName("opni-syncer"),
						HavePorts("syncer-port"),
						HaveEnv("FOO", "BAR"),
						HaveVolumeMounts("opni-alertmanager-data"),
					)),
					HaveMatchingContainer(And(
						HaveName("opni-alertmanager"),
						HavePorts("opni-port", "web-port", "cluster-port"),
						HaveEnv("FOO", "BAR"),
						HaveVolumeMounts("opni-alertmanager-data"),
					)),
				))

				Eventually(Object(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      shared.AlertmanagerService,
						Namespace: gateway.Namespace,
					},
				})).Should(Exist())

				Expect(List(&appsv1.StatefulSetList{}, &client.ListOptions{
					Namespace: gateway.Namespace,
					LabelSelector: labels.SelectorFromSet(labels.Set{
						"app.kubernetes.io/name": "opni-alerting",
					}),
				})()).To(HaveLen(1))
			})
		})

		When("using HA mode", func() {
			It("should create the alerting resources", func() {
				cl := &corev1beta1.AlertingCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "alerting",
						Namespace: gateway.Namespace,
					},
					Spec: corev1beta1.AlertingClusterSpec{
						Gateway: corev1.LocalObjectReference{
							Name: gateway.Name,
						},
						Alertmanager: corev1beta1.AlertManagerSpec{
							Enable: true,
							ApplicationSpec: corev1beta1.AlertingApplicationSpec{
								Replicas: lo.ToPtr(int32(3)),
								ExtraEnvVars: []corev1.EnvVar{
									{
										Name:  "FOO",
										Value: "BAR",
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), cl)).To(Succeed())

				Eventually(Object(&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      shared.AlertmanagerService,
						Namespace: gateway.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(cl),
					HaveReplicaCount(3),
					HaveMatchingContainer(And(
						HaveName("opni-syncer"),
						HavePorts("syncer-port"),
						HaveEnv("FOO", "BAR"),
						HaveVolumeMounts("opni-alertmanager-data"),
					)),
					HaveMatchingContainer(And(
						HaveName("opni-alertmanager"),
						HavePorts("opni-port", "web-port", "cluster-port"),
						HaveEnv("FOO", "BAR"),
						HaveVolumeMounts("opni-alertmanager-data"),
					)),
				))

				Eventually(Object(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      shared.AlertmanagerService,
						Namespace: gateway.Namespace,
					},
				})).Should(Exist())

				Expect(List(&appsv1.StatefulSetList{}, &client.ListOptions{
					Namespace: gateway.Namespace,
					LabelSelector: labels.SelectorFromSet(labels.Set{
						"app.kubernetes.io/name": "opni-alerting",
					}),
				})()).To(HaveLen(1))
			})
		})
	})
})
