package kubernetes_manager_test

import (
	"context"
	"fmt"
	"reflect"
	"time"

	opsterv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"
	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/test/testlog"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/apis/loggingadmin"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/management/kubernetes_manager"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	giBytes = 1073741824
)

var (
	defaultIndices = []string{
		"logs*",
		"some-other-index",
	}
)

func createRequest() *loggingadmin.OpensearchClusterV2 {
	return &loggingadmin.OpensearchClusterV2{
		ExternalURL:   "https://test.example.com",
		DataRetention: lo.ToPtr("7d"),
		DataNodes: &loggingadmin.DataDetails{
			Replicas:           lo.ToPtr(int32(3)),
			DiskSize:           "10Gi",
			MemoryLimit:        "2Gi",
			EnableAntiAffinity: lo.ToPtr(false),
		},
		Dashboards: &loggingadmin.DashboardsDetails{
			Enabled:  lo.ToPtr(true),
			Replicas: lo.ToPtr(int32(1)),
		},
	}
}

var _ = Describe("Opensearch Admin V2", Ordered, Label("integration"), func() {
	var (
		namespace         string
		manager           *kubernetes_manager.KubernetesManagerDriver
		dashboards        opsterv1.DashboardsConfig
		security          *opsterv1.Security
		version           string
		opensearchVersion string

		timeout  = 30 * time.Second
		interval = time.Second

		nats = "opni"
	)

	BeforeEach(func() {
		namespace = "test-logging-v2"
		version = "0.12.1"
		opensearchVersion = "2.8.0"

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
				Image: lo.ToPtr("docker.io/rancher/opensearch-dashboards:v0.12.1-2.8.0"),
			},
			Replicas: 1,
			Enable:   true,
			Version:  opensearchVersion,
			Tls: &opsterv1.DashboardsTlsConfig{
				Enable:   true,
				Generate: true,
			},
			Service: opsterv1.DashboardsServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
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
		var err error
		manager, err = kubernetes_manager.NewKubernetesManagerDriver(
			kubernetes_manager.KubernetesManagerDriverOptions{
				K8sClient:         k8sClient,
				OpensearchCluster: opniCluster,
				Logger:            testlog.Log,
			},
		)
		Expect(err).NotTo(HaveOccurred())
	})

	When("opniopensearch object does not exist", func() {
		Specify("get should succeed and return nothing", func() {
			object, err := manager.GetCluster(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(object).To(Equal(&loggingadmin.OpensearchClusterV2{}))
		})
		Specify("delete should error", func() {
			err := manager.DeleteCluster(context.Background())
			Expect(err).To(HaveOccurred())
		})

		Context("creating an opensearch cluster", func() {
			When("it has no separate roles and 3 replicas", func() {
				Context("no options are enabled", func() {
					request := createRequest()
					object := &loggingv1beta1.OpniOpensearch{}
					Specify("put should succeed", func() {
						err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should not create a credential secret", func() {
						Consistently(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      "opni-opensearch-s3",
								Namespace: namespace,
							}, &corev1.Secret{})
						}, timeout, interval).ShouldNot(Succeed())
					})
					It("should create a single node pool", func() {
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      "opni",
								Namespace: namespace,
							}, object)
						}, timeout, interval).Should(Succeed())
						Expect(object.Spec.Version).To(Equal(version))
						Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
						Expect(object.Spec.IndexRetention).To(Equal("7d"))
						Expect(object.Spec.Security).To(Equal(security))
						Expect(object.Spec.Dashboards).To(Equal(dashboards))
						Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
							Component: "data",
							Replicas:  3,
							DiskSize:  "10Gi",
							Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
							Roles: []string{
								"data",
								"ingest",
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
							Labels: map[string]string{
								kubernetes_manager.LabelOpniNodeGroup: "data",
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DISABLE_INSTALL_DEMO_CONFIG",
									Value: "true",
								},
							},
						}))
						Expect(len(object.Spec.NodePools)).To(Equal(1))
						Expect(object.Spec.OpensearchSettings.S3Settings).To(BeNil())
					})
					Specify("cleanup", func() {
						Expect(k8sClient.Delete(context.Background(), object)).To(Succeed())
					})
				})
				Context("s3 settings are enabled", func() {
					object := &loggingv1beta1.OpniOpensearch{}
					request := createRequest()
					request.S3 = &loggingadmin.OpensearchS3Settings{
						Endpoint: "s3.example.com",
						Credentials: &loggingadmin.S3Credentials{
							AccessKey: "testaccesskey",
							SecretKey: "testsecretkey",
						},
						Bucket: "testbucket",
						Folder: lo.ToPtr("backups"),
					}
					Specify("put should succeed", func() {
						err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should apply s3 settings", func() {
						Eventually(Object(&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "opni-opensearch-s3",
								Namespace: namespace,
							},
						})).Should(ExistAnd(
							HaveData(
								"accessKey", "testaccesskey",
								"secretKey", "testsecretkey",
							),
						))
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      "opni",
								Namespace: namespace,
							}, object)
						}, timeout, interval).Should(Succeed())
						Expect(object.Spec.OpensearchSettings.S3Settings).To(Equal(&loggingv1beta1.OpensearchS3Settings{
							Endpoint: "s3.example.com",
							Protocol: loggingv1beta1.OpensearchS3ProtocolHTTPS,
							CredentialSecret: corev1.LocalObjectReference{
								Name: "opni-opensearch-s3",
							},
							Repository: loggingv1beta1.S3PathSettings{
								Bucket: "testbucket",
								Folder: "backups",
							},
						}))
					})
				})
				Context("anti affinity is enabled", func() {
					object := &loggingv1beta1.OpniOpensearch{}
					request := createRequest()
					request.DataNodes.EnableAntiAffinity = lo.ToPtr(true)
					Specify("put should succeed", func() {
						err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should create a single node pool with anti affinity", func() {
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      "opni",
								Namespace: namespace,
							}, object)
						}, timeout, interval).Should(Succeed())
						Expect(object.Spec.Version).To(Equal(version))
						Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
						Expect(object.Spec.IndexRetention).To(Equal("7d"))
						Expect(object.Spec.Security).To(Equal(security))
						Expect(object.Spec.Dashboards).To(Equal(dashboards))
						Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
							Component: "data",
							Replicas:  3,
							DiskSize:  "10Gi",
							Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
							Roles: []string{
								"data",
								"ingest",
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
							Labels: map[string]string{
								kubernetes_manager.LabelOpniNodeGroup: "data",
							},
							Affinity: &corev1.Affinity{
								PodAntiAffinity: &corev1.PodAntiAffinity{
									PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
										{
											Weight: 100,
											PodAffinityTerm: corev1.PodAffinityTerm{
												LabelSelector: &metav1.LabelSelector{
													MatchLabels: map[string]string{
														kubernetes_manager.LabelOpsterCluster: "opni",
														kubernetes_manager.LabelOpniNodeGroup: "data",
													},
												},
												TopologyKey: kubernetes_manager.TopologyKeyK8sHost,
											},
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DISABLE_INSTALL_DEMO_CONFIG",
									Value: "true",
								},
							},
						}))
						Expect(len(object.Spec.NodePools)).To(Equal(1))
					})
					Specify("cleanup", func() {
						Expect(k8sClient.Delete(context.Background(), object)).To(Succeed())
					})
				})
				Context("data persistence is disabled", func() {
					request := createRequest()
					request.DataNodes.Persistence = &loggingadmin.DataPersistence{
						Enabled: lo.ToPtr(false),
					}
					object := &loggingv1beta1.OpniOpensearch{}
					Specify("put should succeed", func() {
						err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should create a single node pool", func() {
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      "opni",
								Namespace: namespace,
							}, object)
						}, timeout, interval).Should(Succeed())
						Expect(object.Spec.Version).To(Equal(version))
						Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
						Expect(object.Spec.IndexRetention).To(Equal("7d"))
						Expect(object.Spec.Security).To(Equal(security))
						Expect(object.Spec.Dashboards).To(Equal(dashboards))
						Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
							Component: "data",
							Replicas:  3,
							DiskSize:  "10Gi",
							Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
							Roles: []string{
								"data",
								"ingest",
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
							Labels: map[string]string{
								kubernetes_manager.LabelOpniNodeGroup: "data",
							},
							Persistence: &opsterv1.PersistenceConfig{
								PersistenceSource: opsterv1.PersistenceSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DISABLE_INSTALL_DEMO_CONFIG",
									Value: "true",
								},
							},
						}))
						Expect(len(object.Spec.NodePools)).To(Equal(1))
					})
					Specify("cleanup", func() {
						Expect(k8sClient.Delete(context.Background(), object)).To(Succeed())
					})
				})
				Context("data persistence is explicitly specified", func() {
					request := createRequest()
					request.DataNodes.Persistence = &loggingadmin.DataPersistence{
						Enabled:      lo.ToPtr(true),
						StorageClass: lo.ToPtr("testclass"),
					}
					object := &loggingv1beta1.OpniOpensearch{}
					Specify("put should succeed", func() {
						err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should create a single node pool", func() {
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      "opni",
								Namespace: namespace,
							}, object)
						}, timeout, interval).Should(Succeed())
						Expect(object.Spec.Version).To(Equal(version))
						Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
						Expect(object.Spec.IndexRetention).To(Equal("7d"))
						Expect(object.Spec.Security).To(Equal(security))
						Expect(object.Spec.Dashboards).To(Equal(dashboards))
						Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
							Component: "data",
							Replicas:  3,
							DiskSize:  "10Gi",
							Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
							Roles: []string{
								"data",
								"ingest",
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
							Labels: map[string]string{
								kubernetes_manager.LabelOpniNodeGroup: "data",
							},
							Persistence: &opsterv1.PersistenceConfig{
								PersistenceSource: opsterv1.PersistenceSource{
									PVC: &opsterv1.PVCSource{
										StorageClassName: "testclass",
										AccessModes: []corev1.PersistentVolumeAccessMode{
											corev1.ReadWriteOnce,
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DISABLE_INSTALL_DEMO_CONFIG",
									Value: "true",
								},
							},
						}))
					})
					Specify("cleanup", func() {
						Expect(k8sClient.Delete(context.Background(), object)).To(Succeed())
					})
				})
				Context("cpu resources are specified", func() {
					request := createRequest()
					request.DataNodes.CpuResources = &loggingadmin.CPUResource{
						Request: "100m",
						Limit:   "150m",
					}
					object := &loggingv1beta1.OpniOpensearch{}
					Specify("put should succeed", func() {
						err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should create a single node pool", func() {
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      "opni",
								Namespace: namespace,
							}, object)
						}, timeout, interval).Should(Succeed())
						Expect(object.Spec.Version).To(Equal(version))
						Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
						Expect(object.Spec.IndexRetention).To(Equal("7d"))
						Expect(object.Spec.Security).To(Equal(security))
						Expect(object.Spec.Dashboards).To(Equal(dashboards))
						Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
							Component: "data",
							Replicas:  3,
							DiskSize:  "10Gi",
							Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
							Roles: []string{
								"data",
								"ingest",
								"master",
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("150m"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
							},
							Labels: map[string]string{
								kubernetes_manager.LabelOpniNodeGroup: "data",
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DISABLE_INSTALL_DEMO_CONFIG",
									Value: "true",
								},
							},
						}))
						Expect(len(object.Spec.NodePools)).To(Equal(1))
					})
					Specify("cleanup", func() {
						Expect(k8sClient.Delete(context.Background(), object)).To(Succeed())
					})
				})
			})
			When("it has an even number of nodes < 5", func() {
				request := createRequest()
				request.DataNodes.Replicas = lo.ToPtr(int32(2))
				object := &loggingv1beta1.OpniOpensearch{}
				Specify("put should succeed", func() {
					err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create a data and quorum node pool", func() {
					Eventually(func() error {
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      "opni",
							Namespace: namespace,
						}, object)
					}, timeout, interval).Should(Succeed())
					Expect(object.Spec.Version).To(Equal(version))
					Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
					Expect(object.Spec.IndexRetention).To(Equal("7d"))
					Expect(object.Spec.Security).To(Equal(security))
					Expect(object.Spec.Dashboards).To(Equal(dashboards))
					Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
						Component: "data",
						Replicas:  2,
						DiskSize:  "10Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
						Roles: []string{
							"data",
							"ingest",
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
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "data",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
						},
					}))
					Expect(object.Spec.NodePools[1]).To(Equal(opsterv1.NodePool{
						Component: "quorum",
						Replicas:  1,
						DiskSize:  "5Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes/4, giBytes/4),
						Roles: []string{
							"master",
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("640Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("640Mi"),
								corev1.ResourceCPU:    resource.MustParse("100m"),
							},
						},
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "data",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
							{
								Name:  "DISABLE_PERFORMANCE_ANALYZER_AGENT_CLI",
								Value: "true",
							},
						},
					}))
					Expect(len(object.Spec.NodePools)).To(Equal(2))
				})
				Specify("cleanup", func() {
					Expect(k8sClient.Delete(context.Background(), object)).To(Succeed())
				})
			})
			When("it has a number of nodes > 5", func() {
				request := createRequest()
				request.DataNodes.Replicas = lo.ToPtr(int32(7))
				object := &loggingv1beta1.OpniOpensearch{}
				Specify("put should succeed", func() {
					err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create a data pool for the excess nodes", func() {
					Eventually(func() error {
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      "opni",
							Namespace: namespace,
						}, object)
					}, timeout, interval).Should(Succeed())
					Expect(object.Spec.Version).To(Equal(version))
					Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
					Expect(object.Spec.IndexRetention).To(Equal("7d"))
					Expect(object.Spec.Security).To(Equal(security))
					Expect(object.Spec.Dashboards).To(Equal(dashboards))
					Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
						Component: "data",
						Replicas:  5,
						DiskSize:  "10Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
						Roles: []string{
							"data",
							"ingest",
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
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "data",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
						},
					}))
					Expect(object.Spec.NodePools[1]).To(Equal(opsterv1.NodePool{
						Component: "datax",
						Replicas:  2,
						DiskSize:  "10Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
						Roles: []string{
							"data",
							"ingest",
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "data",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
						},
					}))
					Expect(len(object.Spec.NodePools)).To(Equal(2))
				})
				Specify("cleanup", func() {
					Expect(k8sClient.Delete(context.Background(), object)).To(Succeed())
				})
			})
			When("it has separate ingest nodes", func() {
				request := createRequest()
				request.IngestNodes = &loggingadmin.IngestDetails{
					Replicas:    lo.ToPtr(int32(2)),
					MemoryLimit: "4Gi",
				}
				object := &loggingv1beta1.OpniOpensearch{}
				Specify("put should succeed", func() {
					err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create a data and ingest node pool", func() {
					Eventually(func() error {
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      "opni",
							Namespace: namespace,
						}, object)
					}, timeout, interval).Should(Succeed())
					Expect(object.Spec.Version).To(Equal(version))
					Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
					Expect(object.Spec.IndexRetention).To(Equal("7d"))
					Expect(object.Spec.Security).To(Equal(security))
					Expect(object.Spec.Dashboards).To(Equal(dashboards))
					Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
						Component: "data",
						Replicas:  3,
						DiskSize:  "10Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
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
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "data",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
						},
					}))
					Expect(object.Spec.NodePools[1]).To(Equal(opsterv1.NodePool{
						Component: "ingest",
						Replicas:  2,
						DiskSize:  "5Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes*2, giBytes*2),
						Roles: []string{
							"ingest",
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "ingest",
						},
						Persistence: &opsterv1.PersistenceConfig{
							PersistenceSource: opsterv1.PersistenceSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
						},
					}))
					Expect(len(object.Spec.NodePools)).To(Equal(2))
				})
				Specify("cleanup", func() {
					Expect(k8sClient.Delete(context.Background(), object)).To(Succeed())
				})
			})
			When("it has separate controlplane nodes", func() {
				request := createRequest()
				request.ControlplaneNodes = &loggingadmin.ControlplaneDetails{
					Replicas: lo.ToPtr(int32(3)),
				}
				object := &loggingv1beta1.OpniOpensearch{}
				Specify("put should succeed", func() {
					err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create a data and controlplane", func() {
					Eventually(func() error {
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      "opni",
							Namespace: namespace,
						}, object)
					}, timeout, interval).Should(Succeed())
					Expect(object.Spec.Version).To(Equal(version))
					Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
					Expect(object.Spec.IndexRetention).To(Equal("7d"))
					Expect(object.Spec.Security).To(Equal(security))
					Expect(object.Spec.Dashboards).To(Equal(dashboards))
					Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
						Component: "data",
						Replicas:  3,
						DiskSize:  "10Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
						Roles: []string{
							"data",
							"ingest",
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "data",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
						},
					}))
					Expect(object.Spec.NodePools[1]).To(Equal(opsterv1.NodePool{
						Component: "controlplane",
						Replicas:  3,
						DiskSize:  "5Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes/4, giBytes/4),
						Roles: []string{
							"master",
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("640Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("640Mi"),
								corev1.ResourceCPU:    resource.MustParse("100m"),
							},
						},
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "controlplane",
						},
						Affinity: &corev1.Affinity{
							PodAntiAffinity: &corev1.PodAntiAffinity{
								PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
									{
										Weight: 100,
										PodAffinityTerm: corev1.PodAffinityTerm{
											LabelSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{
													kubernetes_manager.LabelOpsterCluster: "opni",
													kubernetes_manager.LabelOpniNodeGroup: "controlplane",
												},
											},
											TopologyKey: kubernetes_manager.TopologyKeyK8sHost,
										},
									},
								},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
							{
								Name:  "DISABLE_PERFORMANCE_ANALYZER_AGENT_CLI",
								Value: "true",
							},
						},
					}))
					Expect(len(object.Spec.NodePools)).To(Equal(2))
				})
				Specify("cleanup", func() {
					Expect(k8sClient.Delete(context.Background(), object)).To(Succeed())
				})
			})
			When("it has separate controlplane nodes with persistence", func() {
				request := createRequest()
				request.ControlplaneNodes = &loggingadmin.ControlplaneDetails{
					Replicas: lo.ToPtr(int32(3)),
					Persistence: &loggingadmin.DataPersistence{
						Enabled:      lo.ToPtr(true),
						StorageClass: lo.ToPtr("testclass"),
					},
				}
				object := &loggingv1beta1.OpniOpensearch{}
				Specify("put should succeed", func() {
					err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create a data and controlplane", func() {
					Eventually(func() error {
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      "opni",
							Namespace: namespace,
						}, object)
					}, timeout, interval).Should(Succeed())
					Expect(object.Spec.Version).To(Equal(version))
					Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
					Expect(object.Spec.IndexRetention).To(Equal("7d"))
					Expect(object.Spec.Security).To(Equal(security))
					Expect(object.Spec.Dashboards).To(Equal(dashboards))
					Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
						Component: "data",
						Replicas:  3,
						DiskSize:  "10Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
						Roles: []string{
							"data",
							"ingest",
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "data",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
						},
					}))
					Expect(object.Spec.NodePools[1]).To(Equal(opsterv1.NodePool{
						Component: "controlplane",
						Replicas:  3,
						DiskSize:  "5Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes/4, giBytes/4),
						Roles: []string{
							"master",
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("640Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("640Mi"),
								corev1.ResourceCPU:    resource.MustParse("100m"),
							},
						},
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "controlplane",
						},
						Affinity: &corev1.Affinity{
							PodAntiAffinity: &corev1.PodAntiAffinity{
								PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
									{
										Weight: 100,
										PodAffinityTerm: corev1.PodAffinityTerm{
											LabelSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{
													kubernetes_manager.LabelOpsterCluster: "opni",
													kubernetes_manager.LabelOpniNodeGroup: "controlplane",
												},
											},
											TopologyKey: kubernetes_manager.TopologyKeyK8sHost,
										},
									},
								},
							},
						},
						Persistence: &opsterv1.PersistenceConfig{
							PersistenceSource: opsterv1.PersistenceSource{
								PVC: &opsterv1.PVCSource{
									StorageClassName: "testclass",
									AccessModes: []corev1.PersistentVolumeAccessMode{
										corev1.ReadWriteOnce,
									},
								},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
							{
								Name:  "DISABLE_PERFORMANCE_ANALYZER_AGENT_CLI",
								Value: "true",
							},
						},
					}))
					Expect(len(object.Spec.NodePools)).To(Equal(2))
				})
				Specify("cleanup", func() {
					Expect(k8sClient.Delete(context.Background(), object)).To(Succeed())
				})
			})
		})
	})
	Context("opniopensearch object does exist", func() {
		request := createRequest()
		request.DataNodes.Persistence = &loggingadmin.DataPersistence{
			Enabled:      lo.ToPtr(true),
			StorageClass: lo.ToPtr("testclass"),
		}
		object := &loggingv1beta1.OpniOpensearch{}
		Specify("put should succeed", func() {
			err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should create a single node pool", func() {
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "opni",
					Namespace: namespace,
				}, object)
			}, timeout, interval).Should(Succeed())
			Expect(object.Spec.Version).To(Equal(version))
			Expect(object.Spec.OpensearchVersion).To(Equal(opensearchVersion))
			Expect(object.Spec.IndexRetention).To(Equal("7d"))
			Expect(object.Spec.Security).To(Equal(security))
			Expect(object.Spec.Dashboards).To(Equal(dashboards))
			Expect(object.Spec.NodePools[0]).To(Equal(opsterv1.NodePool{
				Component: "data",
				Replicas:  3,
				DiskSize:  "10Gi",
				Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes, giBytes),
				Roles: []string{
					"data",
					"ingest",
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
				Labels: map[string]string{
					kubernetes_manager.LabelOpniNodeGroup: "data",
				},
				Persistence: &opsterv1.PersistenceConfig{
					PersistenceSource: opsterv1.PersistenceSource{
						PVC: &opsterv1.PVCSource{
							StorageClassName: "testclass",
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
						},
					},
				},
				Env: []corev1.EnvVar{
					{
						Name:  "DISABLE_INSTALL_DEMO_CONFIG",
						Value: "true",
					},
				},
			}))
			Expect(len(object.Spec.NodePools)).To(Equal(1))
		})
		Specify("get should return the object", func() {
			object, err := manager.GetCluster(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(object).To(Equal(request))
		})
		When("updating the cluster", func() {
			BeforeEach(func() {
				version = "12.0.0"
			})
			newRequest := createRequest()
			newRequest.DataNodes.Persistence = &loggingadmin.DataPersistence{
				Enabled:      lo.ToPtr(true),
				StorageClass: lo.ToPtr("testclass"),
			}
			newRequest.ControlplaneNodes = &loggingadmin.ControlplaneDetails{
				Replicas: lo.ToPtr(int32(3)),
				Persistence: &loggingadmin.DataPersistence{
					Enabled:      lo.ToPtr(true),
					StorageClass: lo.ToPtr("testclass"),
				},
			}
			It("should succeed and update the cluster, excluding the version", func() {
				err := manager.CreateOrUpdateCluster(context.Background(), newRequest, version, nats)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      "opni",
						Namespace: namespace,
					}, object)
					if err != nil {
						return false
					}
					if len(object.Spec.NodePools) < 2 {
						return false
					}
					return reflect.DeepEqual(object.Spec.NodePools[1], opsterv1.NodePool{
						Component: "controlplane",
						Replicas:  3,
						DiskSize:  "5Gi",
						Jvm:       fmt.Sprintf("-Xmx%d -Xms%d", giBytes/4, giBytes/4),
						Roles: []string{
							"master",
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("640Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("640Mi"),
								corev1.ResourceCPU:    resource.MustParse("100m"),
							},
						},
						Labels: map[string]string{
							kubernetes_manager.LabelOpniNodeGroup: "controlplane",
						},
						Affinity: &corev1.Affinity{
							PodAntiAffinity: &corev1.PodAntiAffinity{
								PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
									{
										Weight: 100,
										PodAffinityTerm: corev1.PodAffinityTerm{
											LabelSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{
													kubernetes_manager.LabelOpsterCluster: "opni",
													kubernetes_manager.LabelOpniNodeGroup: "controlplane",
												},
											},
											TopologyKey: kubernetes_manager.TopologyKeyK8sHost,
										},
									},
								},
							},
						},
						Persistence: &opsterv1.PersistenceConfig{
							PersistenceSource: opsterv1.PersistenceSource{
								PVC: &opsterv1.PVCSource{
									StorageClassName: "testclass",
									AccessModes: []corev1.PersistentVolumeAccessMode{
										corev1.ReadWriteOnce,
									},
								},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "DISABLE_INSTALL_DEMO_CONFIG",
								Value: "true",
							},
							{
								Name:  "DISABLE_PERFORMANCE_ANALYZER_AGENT_CLI",
								Value: "true",
							},
						},
					})
				}, timeout, interval).Should(BeTrue())
				Expect(object.Spec.Security).To(Equal(security))
				Expect(object.Spec.Version).To(Equal("0.12.1"))
				Expect(len(object.Spec.NodePools)).To(Equal(2))
			})
			When("upgrade is available", func() {
				Specify("setup status", func() {
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      "opni",
						Namespace: namespace,
					}, object)
					Expect(err).NotTo(HaveOccurred())
					object.Status.OpensearchVersion = lo.ToPtr("2.8.0")
					object.Status.Version = lo.ToPtr("0.12.1")
					Expect(k8sClient.Status().Update(context.Background(), object)).To(Succeed())
				})
				Specify("check upgrade available should return true", func() {
					response, err := manager.UpgradeAvailable(context.Background(), version)
					Expect(err).NotTo(HaveOccurred())
					Expect(response).To(BeTrue())
				})
				Specify("do upgrade should upgrade the version", func() {
					err := manager.DoUpgrade(context.Background(), version)
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      "opni",
							Namespace: namespace,
						}, object)
						if err != nil {
							return false
						}
						return object.Spec.Version == "12.0.0"
					}, timeout, interval).Should(BeTrue())
				})
			})
		})

		Specify("delete should succeed", func() {
			err := manager.DeleteCluster(context.Background())
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
	Context("snapshots", Ordered, func() {
		var repo *loggingv1beta1.OpensearchRepository
		Context("repository does not exist", func() {
			When("creating a new recurring snapshot", func() {
				It("should not succeed", func() {
					err := manager.CreateOrUpdateSnapshotSchedule(context.Background(), &loggingadmin.SnapshotSchedule{
						Ref: &loggingadmin.SnapshotReference{
							Name: "test",
						},
					}, defaultIndices)
					Expect(err).To(HaveOccurred())
				})
			})
		})
		Context("repository does exist", Ordered, func() {
			BeforeAll(func() {
				request := createRequest()
				err := manager.CreateOrUpdateCluster(context.Background(), request, version, nats)
				Expect(err).NotTo(HaveOccurred())
			})
			AfterAll(func() {
				err := manager.DeleteCluster(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      "opni",
						Namespace: namespace,
					}, &loggingv1beta1.OpniOpensearch{})
					if err != nil {
						return k8serrors.IsNotFound(err)
					}
					return false
				}, timeout, interval).Should(BeTrue())
				err = k8sClient.Delete(context.Background(), &loggingv1beta1.OpensearchRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni",
						Namespace: namespace,
					},
					Spec: loggingv1beta1.OpensearchRepositorySpec{
						Settings: loggingv1beta1.RepositorySettings{
							S3: &loggingv1beta1.S3PathSettings{
								Bucket: "test-bucket",
								Folder: "test",
							},
						},
						OpensearchClusterRef: &opnimeta.OpensearchClusterRef{
							Name:      "opni",
							Namespace: namespace,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.DeleteAllOf(context.Background(), &loggingv1beta1.RecurringSnapshot{}, client.InNamespace(namespace))
				Expect(err).NotTo(HaveOccurred())
			})
			JustBeforeEach(func() {
				repo = &loggingv1beta1.OpensearchRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni",
						Namespace: namespace,
					},
					Spec: loggingv1beta1.OpensearchRepositorySpec{
						Settings: loggingv1beta1.RepositorySettings{
							S3: &loggingv1beta1.S3PathSettings{
								Bucket: "test-bucket",
								Folder: "test",
							},
						},
						OpensearchClusterRef: &opnimeta.OpensearchClusterRef{
							Name:      "opni",
							Namespace: namespace,
						},
					},
				}
				err := k8sClient.Create(context.Background(), repo)
				Expect(client.IgnoreAlreadyExists(err)).NotTo(HaveOccurred())
			})
			When("creating a new recurring snapshot", func() {
				It("should succeed", func() {
					err := manager.CreateOrUpdateSnapshotSchedule(context.Background(), &loggingadmin.SnapshotSchedule{
						Ref: &loggingadmin.SnapshotReference{
							Name: "test-recurring",
						},
						CronSchedule: "00 * * * *",
					}, defaultIndices)
					Expect(err).NotTo(HaveOccurred())
					Eventually(Object(&loggingv1beta1.RecurringSnapshot{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-recurring",
							Namespace: namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(repo),
					))
				})
			})
			When("fetching snapshots", func() {
				It("should succeed", func() {
					s, err := manager.GetSnapshotSchedule(context.Background(), &loggingadmin.SnapshotReference{
						Name: "test-recurring",
					}, defaultIndices)
					Expect(err).NotTo(HaveOccurred())
					Expect(s.GetCronSchedule()).To(Equal("00 * * * *"))
				})
			})
			When("extra indices are defined", Ordered, func() {
				It("should successfully update the snapshot", func() {
					err := manager.CreateOrUpdateSnapshotSchedule(context.Background(), &loggingadmin.SnapshotSchedule{
						Ref: &loggingadmin.SnapshotReference{
							Name: "test-recurring",
						},
						CronSchedule: "00 * * * *",
						AdditionalIndices: []string{
							"test",
						},
					}, defaultIndices)
					Expect(err).NotTo(HaveOccurred())
					s := &loggingv1beta1.RecurringSnapshot{}
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      "test-recurring",
							Namespace: namespace,
						}, s)
						return err == nil
					}).Should(BeTrue())
					Expect(s.Spec.Snapshot.Indices).To(ContainElements("test", "logs*"))
				})
				It("should successfully create the snapshot", func() {
					err := manager.CreateOrUpdateSnapshotSchedule(context.Background(), &loggingadmin.SnapshotSchedule{
						Ref: &loggingadmin.SnapshotReference{
							Name: "test-indices",
						},
						CronSchedule: "00 * * * *",
						AdditionalIndices: []string{
							"foo",
							"bar",
						},
					}, defaultIndices)
					Expect(err).NotTo(HaveOccurred())
					s := &loggingv1beta1.RecurringSnapshot{}
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      "test-indices",
							Namespace: namespace,
						}, s)
						return err == nil
					}).Should(BeTrue())
					Expect(s.Spec.Snapshot.Indices).To(ContainElements("foo", "bar", "logs*"))
				})
				It("should only return the extra indices when fetched", func() {
					s, err := manager.GetSnapshotSchedule(context.Background(), &loggingadmin.SnapshotReference{
						Name: "test-recurring",
					}, defaultIndices)
					Expect(err).NotTo(HaveOccurred())
					Expect(s.GetAdditionalIndices()).To(Equal([]string{"test"}))
				})
			})
			When("listing snapshots", func() {
				It("should find all the snapshots", func() {
					list, err := manager.ListAllSnapshotSchedules(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(len(list.GetStatuses())).To(Equal(2))
				})
			})
			When("deleting snapshots", func() {
				It("should delete recurring snapshots", func() {
					err := manager.DeleteSnapshotSchedule(context.Background(), &loggingadmin.SnapshotReference{
						Name: "test-recurring",
					})
					Expect(err).NotTo(HaveOccurred())
					list, err := manager.ListAllSnapshotSchedules(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(len(list.GetStatuses())).To(Equal(1))
				})
			})
		})
	})

})
