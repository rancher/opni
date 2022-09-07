package controllers

import (
	"context"
	"fmt"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	opsterv1 "opensearch.opster.io/api/v1"
)

var _ = Describe("Logging OpniOpensearch Controller", Ordered, Label("controller"), func() {
	var (
		testNs string
		object *loggingv1beta1.OpniOpensearch
	)
	Specify("setup", func() {
		testNs = makeTestNamespace()
		object = &loggingv1beta1.OpniOpensearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni",
				Namespace: testNs,
			},
			Spec: loggingv1beta1.OpniOpensearchSpec{
				ClusterConfigSpec: &loggingv1beta1.ClusterConfigSpec{
					IndexRetention: "7d",
				},
				ExternalURL:       "https://test.example.com",
				Version:           "1.0.0",
				OpensearchVersion: "1.0.0",
				ImageRepo:         "docker.io/rancher",
				OpensearchSettings: loggingv1beta1.OpensearchSettings{
					NodePools: []opsterv1.NodePool{
						{
							Component: "test",
							Replicas:  3,
							DiskSize:  "50Gi",
							Jvm:       fmt.Sprintf("-Xmx%s -Xms%s", "1G", "1G"),
							Roles: []string{
								"data",
								"master",
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
							Persistence: &opsterv1.PersistenceConfig{
								PersistenceSource: opsterv1.PersistenceSource{
									PVC: &opsterv1.PVCSource{
										AccessModes: []corev1.PersistentVolumeAccessMode{
											corev1.ReadWriteOnce,
										},
									},
								},
							},
						},
					},
					Security: &opsterv1.Security{
						Tls: &opsterv1.TlsConfig{
							Transport: &opsterv1.TlsConfigTransport{
								Generate: true,
								PerNode:  true,
							},
							Http: &opsterv1.TlsConfigHttp{
								Generate: true,
							},
						},
					},
					Dashboards: opsterv1.DashboardsConfig{
						Replicas: 1,
						Enable:   true,
						Version:  "1.0.0",
					},
				},
			},
		}
	})

	When("creating an opni opensearch object", func() {
		It("should succeed", func() {
			Expect(k8sClient.Create(context.Background(), object)).To(Succeed())
		})
		It("should create the binding object", func() {
			Eventually(Object(&loggingv1beta1.MulticlusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      object.Name,
					Namespace: testNs,
				},
			})).Should(ExistAnd(
				HaveOwner(object),
			))
		})
		It("should create the opensearch object", func() {
			By("checking the object is created")
			Eventually(Object(&opsterv1.OpenSearchCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      object.Name,
					Namespace: testNs,
				},
			})).Should(ExistAnd(
				HaveOwner(object),
			))
			By("checking the config is correct")
			cluster := &opsterv1.OpenSearchCluster{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      object.Name,
				Namespace: testNs,
			}, cluster)).To(Succeed())
			Expect(cluster.Spec.General).To(Equal(
				opsterv1.GeneralConfig{
					ImageSpec: &opsterv1.ImageSpec{
						Image: lo.ToPtr(fmt.Sprintf(
							"%s/opensearch:%s-%s",
							object.Spec.ImageRepo,
							object.Spec.OpensearchVersion,
							object.Spec.Version,
						)),
					},
					Version:          object.Spec.Version,
					ServiceName:      fmt.Sprintf("%s-opensearch-svc", object.Name),
					HttpPort:         9200,
					SetVMMaxMapCount: true,
				},
			))
			Expect(cluster.Spec.Dashboards).Should(Equal(object.Spec.Dashboards))
			Expect(cluster.Spec.NodePools).Should(Equal(object.Spec.NodePools))
		})
	})
})
