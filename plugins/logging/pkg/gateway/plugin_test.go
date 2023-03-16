package gateway_test

import (
	"context"
	"fmt"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	. "github.com/rancher/opni/plugins/logging/pkg/gateway"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	opsterv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	giBytes = 1073741824
)

var _ = Describe("Logging Plugin", Ordered, Label("unit"), func() {
	var (
		namespace         string
		plugin            *Plugin
		request           *loggingadmin.OpensearchCluster
		nodePool          opsterv1.NodePool
		dashboards        opsterv1.DashboardsConfig
		security          *opsterv1.Security
		version           string
		opensearchVersion string

		timeout  = 10 * time.Second
		interval = time.Second
	)

	BeforeEach(func() {
		namespace = "test-logging"
		version = "0.9.0-rc3"
		opensearchVersion = "2.4.0"

		request = &loggingadmin.OpensearchCluster{
			ExternalURL: "https://test.example.com",
			Dashboards: &loggingadmin.DashboardsDetails{
				Enabled:  lo.ToPtr(true),
				Replicas: lo.ToPtr(int32(1)),
			},
			NodePools: []*loggingadmin.OpensearchNodeDetails{
				{
					Name: "test",
					Roles: []string{
						"controlplane",
						"data",
						"ingest",
					},
					DiskSize:    "50Gi",
					MemoryLimit: "2Gi",
					Persistence: &loggingadmin.DataPersistence{
						Enabled: lo.ToPtr(true),
					},
					Replicas:           lo.ToPtr(int32(3)),
					EnableAntiAffinity: lo.ToPtr(true),
				},
			},
			DataRetention: lo.ToPtr("7d"),
		}
		nodePool = opsterv1.NodePool{
			Component: request.NodePools[0].Name,
			Replicas:  3,
			DiskSize:  request.NodePools[0].DiskSize,
			Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
			Roles: []string{
				"master",
				"data",
				"ingest",
			},
			Affinity: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									LabelOpsterCluster:  "opni",
									LabelOpsterNodePool: request.NodePools[0].Name,
								},
							},
							TopologyKey: TopologyKeyK8sHost,
						},
					},
				},
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
		}
		security = &opsterv1.Security{
			Tls: &opsterv1.TlsConfig{
				Transport: &opsterv1.TlsConfigTransport{
					Generate: true,
					PerNode:  true,
				},
				Http: &opsterv1.TlsConfigHttp{
					Generate: true,
				},
			},
		}
		dashboards = opsterv1.DashboardsConfig{
			ImageSpec: &opsterv1.ImageSpec{
				Image: lo.ToPtr("docker.io/rancher/opensearch-dashboards:2.4.0-0.9.0-rc3"),
			},
			Replicas: 1,
			Enable:   true,
			Version:  opensearchVersion,
			Tls: &opsterv1.DashboardsTlsConfig{
				Enable:   true,
				Generate: true,
			},
			AdditionalConfig: map[string]string{
				"opensearchDashboards.branding.logo.defaultUrl":         "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-logo-dark.svg",
				"opensearchDashboards.branding.mark.defaultUrl":         "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-mark.svg",
				"opensearchDashboards.branding.loadingLogo.defaultUrl":  "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-loading.svg",
				"opensearchDashboards.branding.loadingLogo.darkModeUrl": "https://raw.githubusercontent.com/rancher/opni/main/branding/opni-loading-dark.svg",
				"opensearchDashboards.branding.faviconUrl":              "https://raw.githubusercontent.com/rancher/opni/main/branding/favicon.png",
				"opensearchDashboards.branding.applicationTitle":        "Opni Logging",
			},
		}

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(func() error {
			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(ns), &corev1.Namespace{})
			if err != nil {
				if k8serrors.IsNotFound(err) {
					return k8sClient.Create(context.Background(), ns)
				}
				return err
			}
			return nil
		}()).To(Succeed())
	})

	JustBeforeEach(func() {
		opniCluster := &opnimeta.OpensearchClusterRef{
			Name:      "opni",
			Namespace: namespace,
		}
		plugin = NewPlugin(
			context.Background(),
			WithNamespace(namespace),
			WithRestConfig(restConfig),
			WithVersion(version),
			WithOpensearchCluster(opniCluster),
		)
	})

	When("opniopensearch object does not exist", func() {
		Specify("get should succeed and return nothing", func() {
			object, err := plugin.GetOpensearchCluster(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(object).To(Equal(&loggingadmin.OpensearchCluster{}))
		})
		Specify("delete should error", func() {
			_, err := plugin.DeleteOpensearchCluster(context.Background(), nil)
			Expect(err).To(HaveOccurred())
		})
		Context("creating an opensearch cluster", func() {
			When("nodepools don't have memory limits", func() {
				BeforeEach(func() {
					request.NodePools[0].MemoryLimit = ""
				})
				It("should error", func() {
					_, err := plugin.CreateOrUpdateOpensearchCluster(context.Background(), request)
					Expect(err).To(HaveOccurred())
				})
			})
			When("opensearch request is valid", func() {
				It("should succeed", func() {
					_, err := plugin.CreateOrUpdateOpensearchCluster(context.Background(), request)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create the opniopensearch object", func() {
					object := &loggingv1beta1.OpniOpensearch{}
					Eventually(func() error {
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      "opni",
							Namespace: namespace,
						}, object)
					}, timeout, interval).Should(Succeed())
					Expect(object.Spec.Version).To(Equal(version))
					Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
					Expect(object.Spec.IndexRetention).To(Equal("7d"))
					Expect(object.Spec.NodePools[0]).To(Equal(nodePool))
					Expect(object.Spec.Security).To(Equal(security))
					Expect(object.Spec.Dashboards).To(Equal(dashboards))
				})
			})
		})
	})
	When("opniopensearch object does exist", func() {
		object := &loggingv1beta1.OpniOpensearch{}
		Specify("get should return the object", func() {
			object, err := plugin.GetOpensearchCluster(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(object).To(Equal(request))
		})

		When("updating the cluster", func() {
			BeforeEach(func() {
				request.NodePools[0].MemoryLimit = "4Gi"
				nodePool.Jvm = fmt.Sprintf("-Xmx%d -Xms%d", 2*giBytes, 2*giBytes)
				nodePool.Resources.Limits[corev1.ResourceMemory] = resource.MustParse("4Gi")
				nodePool.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("4Gi")
				version = "0.9.0-rc3"
			})
			It("should succeed and update the cluster, excluding the version", func() {
				_, err := plugin.CreateOrUpdateOpensearchCluster(context.Background(), request)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      "opni",
						Namespace: namespace,
					}, object)
					if err != nil {
						return false
					}
					return reflect.DeepEqual(object.Spec.NodePools[0], nodePool)
				}, timeout, interval).Should(BeTrue())
				Expect(object.Spec.Security).To(Equal(security))
				Expect(object.Spec.Version).To(Equal("0.9.0-rc3"))
			})
		})
		Specify("check upgrade available should return false", func() {
			response, err := plugin.UpgradeAvailable(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.UpgradePending).To(BeFalse())
		})
		XWhen("new version is available", func() {
			BeforeEach(func() {
				version = "0.9.0-rc3"
			})
			Specify("upgrade available should return true", func() {
				Expect(version).To(Equal("0.9.0-rc3"))
				response, err := plugin.UpgradeAvailable(context.Background(), nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.UpgradePending).To(BeTrue())
			})
			Specify("do upgrade should upgrade the version", func() {
				_, err := plugin.DoUpgrade(context.Background(), nil)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      "opni",
						Namespace: namespace,
					}, object)
					if err != nil {
						return false
					}
					return object.Spec.Version == version
				}, timeout, interval).Should(BeTrue())
			})
		})
		Specify("delete should succeed", func() {
			_, err := plugin.DeleteOpensearchCluster(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "opni",
					Namespace: namespace,
				}, object)
				if err != nil {
					return k8serrors.IsNotFound(err)
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})
})
