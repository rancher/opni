package aiops_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/plugins/aiops/apis/admin"
	. "github.com/rancher/opni/plugins/aiops/pkg/gateway"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AI Admin", Ordered, Label("integration"), func() {
	var (
		namespace, version string
		plugin             *AIOpsPlugin
	)
	BeforeEach(func() {
		namespace = "test-ai"
		version = "v0.7.0"

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
		plugin = NewPlugin(
			context.Background(),
			WithNamespace(namespace),
			WithVersion(version),
			WithRestConfig(restConfig),
		)
	})

	When("opnicluster does not exist", func() {
		Specify("get should return 404", func() {
			_, err := plugin.GetAISettings(context.Background(), &emptypb.Empty{})
			Expect(err).To(testutil.MatchStatusCode(codes.NotFound))
		})
		Specify("delete should error", func() {
			_, err := plugin.DeleteAISettings(context.Background(), &emptypb.Empty{})
			Expect(err).To(HaveOccurred())
		})
		Specify("check upgrade should error", func() {
			_, err := plugin.UpgradeAvailable(context.Background(), &emptypb.Empty{})
			Expect(err).To(HaveOccurred())
		})
		Specify("do upgrade should error", func() {
			_, err := plugin.DoUpgrade(context.Background(), &emptypb.Empty{})
			Expect(err).To(HaveOccurred())
		})

		Context("creating a new cluster", func() {
			When("no options are specified", func() {
				request := &admin.AISettings{}
				Specify("put should succeed", func() {
					_, err := plugin.PutAISettings(context.Background(), request)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create an opni cluster", func() {
					cluster := &aiv1beta1.OpniCluster{}
					Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      OpniServicesName,
						Namespace: namespace,
					}, cluster)).Should(Succeed())
					Expect(len(cluster.Spec.Services.Inference.PretrainedModels)).To(BeNumerically("==", 0))
					Expect(cluster.Spec.S3).To(Equal(aiv1beta1.S3Spec{
						Internal: &aiv1beta1.InternalSpec{},
					}))
					Expect(cluster.Spec.Services.GPUController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Inference.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.TrainingController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Drain.Workload.Enabled).To(Equal(lo.ToPtr(false)))
				})
				It("should not create pretrained models", func() {
					list := &aiv1beta1.PretrainedModelList{}
					Expect(k8sClient.List(context.Background(), list, client.InNamespace(namespace))).To(Succeed())
					Expect(len(list.Items)).To(BeNumerically("==", 0))
				})
				Specify("get should return an equivalent settings object", func() {
					resp, err := plugin.GetAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(Equal(request))
				})
				Specify("delete should succeed", func() {
					_, err := plugin.DeleteAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      OpniServicesName,
							Namespace: namespace,
						}, &aiv1beta1.OpniCluster{})
						if err == nil {
							return false
						}
						return k8serrors.IsNotFound(err)
					}).Should(BeTrue())
				})
			})
			When("controlplane is specified with no settings", func() {
				request := &admin.AISettings{
					Controlplane: &admin.PretrainedModel{},
				}
				Specify("put should succeed", func() {
					_, err := plugin.PutAISettings(context.Background(), request)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create an opni cluster", func() {
					cluster := &aiv1beta1.OpniCluster{}
					Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      OpniServicesName,
						Namespace: namespace,
					}, cluster)).Should(Succeed())
					Expect(len(cluster.Spec.Services.Inference.PretrainedModels)).To(BeNumerically("==", 1))
					Expect(cluster.Spec.S3).To(Equal(aiv1beta1.S3Spec{
						Internal: &aiv1beta1.InternalSpec{},
					}))
					Expect(cluster.Spec.Services.GPUController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Inference.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.TrainingController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Drain.Workload.Enabled).To(Equal(lo.ToPtr(false)))
				})
				It("should create a control plane pretrained model", func() {
					list := &aiv1beta1.PretrainedModelList{}
					Expect(k8sClient.List(context.Background(), list, client.InNamespace(namespace))).To(Succeed())
					Expect(len(list.Items)).To(BeNumerically("==", 1))
					cp := list.Items[0]
					Expect(cp.Name).To(Equal("opni-model-controlplane"))
					Expect(cp.Spec.Hyperparameters).To(Equal(map[string]intstr.IntOrString{
						"modelThreshold": intstr.FromString("0.6"),
						"minLogTokens":   intstr.FromInt(1),
						"serviceType":    intstr.FromString("control-plane"),
					}))
				})
				Specify("get should return an equivalent settings object", func() {
					resp, err := plugin.GetAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(Equal(request))
				})
				Specify("delete should succeed", func() {
					_, err := plugin.DeleteAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      OpniServicesName,
							Namespace: namespace,
						}, &aiv1beta1.OpniCluster{})
						if err == nil {
							return false
						}
						return k8serrors.IsNotFound(err)
					}).Should(BeTrue())
					Eventually(func() bool {
						list := &aiv1beta1.PretrainedModelList{}
						err := k8sClient.List(context.Background(), list, client.InNamespace(namespace))
						if err != nil {
							return false
						}
						return len(list.Items) == 0
					}).Should(BeTrue())
				})
			})
			When("controlplane is specified with settings", func() {
				request := &admin.AISettings{
					Controlplane: &admin.PretrainedModel{
						HttpSource: lo.ToPtr("https://example.com"),
					},
				}
				Specify("put should succeed", func() {
					_, err := plugin.PutAISettings(context.Background(), request)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create an opni cluster", func() {
					cluster := &aiv1beta1.OpniCluster{}
					Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      OpniServicesName,
						Namespace: namespace,
					}, cluster)).Should(Succeed())
					Expect(len(cluster.Spec.Services.Inference.PretrainedModels)).To(BeNumerically("==", 1))
					Expect(cluster.Spec.S3).To(Equal(aiv1beta1.S3Spec{
						Internal: &aiv1beta1.InternalSpec{},
					}))
					Expect(cluster.Spec.Services.GPUController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Inference.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.TrainingController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Drain.Workload.Enabled).To(Equal(lo.ToPtr(false)))
				})
				It("should create a control plane pretrained model", func() {
					list := &aiv1beta1.PretrainedModelList{}
					Expect(k8sClient.List(context.Background(), list, client.InNamespace(namespace))).To(Succeed())
					Expect(len(list.Items)).To(BeNumerically("==", 1))
					cp := list.Items[0]
					Expect(cp.Name).To(Equal("opni-model-controlplane"))
					Expect(cp.Spec.Hyperparameters).To(Equal(map[string]intstr.IntOrString{
						"modelThreshold": intstr.FromString("0.6"),
						"minLogTokens":   intstr.FromInt(1),
						"serviceType":    intstr.FromString("control-plane"),
					}))
					Expect(cp.Spec.ModelSource).To(Equal(aiv1beta1.ModelSource{
						HTTP: &aiv1beta1.HTTPSource{
							URL: "https://example.com",
						},
					}))
				})
				Specify("get should return an equivalent settings object", func() {
					resp, err := plugin.GetAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(Equal(request))
				})
				Specify("delete should succeed", func() {
					_, err := plugin.DeleteAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      OpniServicesName,
							Namespace: namespace,
						}, &aiv1beta1.OpniCluster{})
						if err == nil {
							return false
						}
						return k8serrors.IsNotFound(err)
					}).Should(BeTrue())
					Eventually(func() bool {
						list := &aiv1beta1.PretrainedModelList{}
						err := k8sClient.List(context.Background(), list, client.InNamespace(namespace))
						if err != nil {
							return false
						}
						return len(list.Items) == 0
					}).Should(BeTrue())
				})
			})
			When("rancher is specified with no settings", func() {
				request := &admin.AISettings{
					Rancher: &admin.PretrainedModel{},
				}
				Specify("put should succeed", func() {
					_, err := plugin.PutAISettings(context.Background(), request)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create an opni cluster", func() {
					cluster := &aiv1beta1.OpniCluster{}
					Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      OpniServicesName,
						Namespace: namespace,
					}, cluster)).Should(Succeed())
					Expect(len(cluster.Spec.Services.Inference.PretrainedModels)).To(BeNumerically("==", 1))
					Expect(cluster.Spec.S3).To(Equal(aiv1beta1.S3Spec{
						Internal: &aiv1beta1.InternalSpec{},
					}))
					Expect(cluster.Spec.Services.GPUController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Inference.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.TrainingController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Drain.Workload.Enabled).To(Equal(lo.ToPtr(false)))
				})
				It("should create a rancher pretrained model", func() {
					list := &aiv1beta1.PretrainedModelList{}
					Expect(k8sClient.List(context.Background(), list, client.InNamespace(namespace))).To(Succeed())
					Expect(len(list.Items)).To(BeNumerically("==", 1))
					cp := list.Items[0]
					Expect(cp.Name).To(Equal("opni-model-rancher"))
					Expect(cp.Spec.Hyperparameters).To(Equal(map[string]intstr.IntOrString{
						"modelThreshold": intstr.FromString("0.6"),
						"minLogTokens":   intstr.FromInt(1),
						"serviceType":    intstr.FromString("rancher"),
					}))
				})
				Specify("get should return an equivalent settings object", func() {
					resp, err := plugin.GetAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(Equal(request))
				})
				Specify("delete should succeed", func() {
					_, err := plugin.DeleteAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      OpniServicesName,
							Namespace: namespace,
						}, &aiv1beta1.OpniCluster{})
						if err == nil {
							return false
						}
						return k8serrors.IsNotFound(err)
					}).Should(BeTrue())
					Eventually(func() bool {
						list := &aiv1beta1.PretrainedModelList{}
						err := k8sClient.List(context.Background(), list, client.InNamespace(namespace))
						if err != nil {
							return false
						}
						return len(list.Items) == 0
					}).Should(BeTrue())
				})
			})
			When("rancher is specified with settings", func() {
				request := &admin.AISettings{
					Rancher: &admin.PretrainedModel{
						ImageSource: lo.ToPtr("docker.io/opni:test"),
					},
				}
				Specify("put should succeed", func() {
					_, err := plugin.PutAISettings(context.Background(), request)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create an opni cluster", func() {
					cluster := &aiv1beta1.OpniCluster{}
					Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      OpniServicesName,
						Namespace: namespace,
					}, cluster)).Should(Succeed())
					Expect(len(cluster.Spec.Services.Inference.PretrainedModels)).To(BeNumerically("==", 1))
					Expect(cluster.Spec.S3).To(Equal(aiv1beta1.S3Spec{
						Internal: &aiv1beta1.InternalSpec{},
					}))
					Expect(cluster.Spec.Services.GPUController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Inference.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.TrainingController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Drain.Workload.Enabled).To(Equal(lo.ToPtr(false)))
				})
				It("should create a rancher pretrained model", func() {
					list := &aiv1beta1.PretrainedModelList{}
					Expect(k8sClient.List(context.Background(), list, client.InNamespace(namespace))).To(Succeed())
					Expect(len(list.Items)).To(BeNumerically("==", 1))
					cp := list.Items[0]
					Expect(cp.Name).To(Equal("opni-model-rancher"))
					Expect(cp.Spec.Hyperparameters).To(Equal(map[string]intstr.IntOrString{
						"modelThreshold": intstr.FromString("0.6"),
						"minLogTokens":   intstr.FromInt(1),
						"serviceType":    intstr.FromString("rancher"),
					}))
					Expect(cp.Spec.ModelSource).To(Equal(aiv1beta1.ModelSource{
						Container: &aiv1beta1.ContainerSource{
							Image: "docker.io/opni:test",
						},
					}))
				})
				Specify("get should return an equivalent settings object", func() {
					resp, err := plugin.GetAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(Equal(request))
				})
				Specify("delete should succeed", func() {
					_, err := plugin.DeleteAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      OpniServicesName,
							Namespace: namespace,
						}, &aiv1beta1.OpniCluster{})
						if err == nil {
							return false
						}
						return k8serrors.IsNotFound(err)
					}).Should(BeTrue())
					Eventually(func() bool {
						list := &aiv1beta1.PretrainedModelList{}
						err := k8sClient.List(context.Background(), list, client.InNamespace(namespace))
						if err != nil {
							return false
						}
						return len(list.Items) == 0
					}).Should(BeTrue())
				})
			})
			When("longhorn is specified with no settings", func() {
				request := &admin.AISettings{
					Longhorn: &admin.PretrainedModel{},
				}
				Specify("put should succeed", func() {
					_, err := plugin.PutAISettings(context.Background(), request)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create an opni cluster", func() {
					cluster := &aiv1beta1.OpniCluster{}
					Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      OpniServicesName,
						Namespace: namespace,
					}, cluster)).Should(Succeed())
					Expect(len(cluster.Spec.Services.Inference.PretrainedModels)).To(BeNumerically("==", 1))
					Expect(cluster.Spec.S3).To(Equal(aiv1beta1.S3Spec{
						Internal: &aiv1beta1.InternalSpec{},
					}))
					Expect(cluster.Spec.Services.GPUController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Inference.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.TrainingController.Enabled).To(Equal(lo.ToPtr(false)))
					Expect(cluster.Spec.Services.Drain.Workload.Enabled).To(Equal(lo.ToPtr(false)))
				})
				It("should create a longhorn pretrained model", func() {
					list := &aiv1beta1.PretrainedModelList{}
					Expect(k8sClient.List(context.Background(), list, client.InNamespace(namespace))).To(Succeed())
					Expect(len(list.Items)).To(BeNumerically("==", 1))
					cp := list.Items[0]
					Expect(cp.Name).To(Equal("opni-model-longhorn"))
					Expect(cp.Spec.Hyperparameters).To(Equal(map[string]intstr.IntOrString{
						"modelThreshold": intstr.FromString("0.8"),
						"minLogTokens":   intstr.FromInt(1),
						"serviceType":    intstr.FromString("longhorn"),
					}))
				})
				Specify("get should return an equivalent settings object", func() {
					resp, err := plugin.GetAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(Equal(request))
				})
				Specify("delete should succeed", func() {
					_, err := plugin.DeleteAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      OpniServicesName,
							Namespace: namespace,
						}, &aiv1beta1.OpniCluster{})
						if err == nil {
							return false
						}
						return k8serrors.IsNotFound(err)
					}).Should(BeTrue())
					Eventually(func() bool {
						list := &aiv1beta1.PretrainedModelList{}
						err := k8sClient.List(context.Background(), list, client.InNamespace(namespace))
						if err != nil {
							return false
						}
						return len(list.Items) == 0
					}).Should(BeTrue())
				})
			})
			When("gpu is configured", func() {
				request := &admin.AISettings{
					GpuSettings: &admin.GPUSettings{},
				}
				Specify("put should succeed", func() {
					_, err := plugin.PutAISettings(context.Background(), request)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should create an opni cluster", func() {
					cluster := &aiv1beta1.OpniCluster{}
					Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      OpniServicesName,
						Namespace: namespace,
					}, cluster)).Should(Succeed())
					Expect(cluster.Spec.S3).To(Equal(aiv1beta1.S3Spec{
						Internal: &aiv1beta1.InternalSpec{},
					}))
					Expect(cluster.Spec.Services.GPUController.Enabled).To(Equal(lo.ToPtr(true)))
					Expect(cluster.Spec.Services.Inference.Enabled).To(Equal(lo.ToPtr(true)))
					Expect(cluster.Spec.Services.TrainingController.Enabled).To(Equal(lo.ToPtr(true)))
					Expect(cluster.Spec.Services.Drain.Workload.Enabled).To(Equal(lo.ToPtr(true)))
				})
				Specify("get should return an equivalent settings object", func() {
					resp, err := plugin.GetAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(Equal(request))
				})
				Specify("delete should succeed", func() {
					_, err := plugin.DeleteAISettings(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      OpniServicesName,
							Namespace: namespace,
						}, &aiv1beta1.OpniCluster{})
						if err == nil {
							return false
						}
						return k8serrors.IsNotFound(err)
					}).Should(BeTrue())
					Eventually(func() bool {
						list := &aiv1beta1.PretrainedModelList{}
						err := k8sClient.List(context.Background(), list, client.InNamespace(namespace))
						if err != nil {
							return false
						}
						return len(list.Items) == 0
					}).Should(BeTrue())
				})
			})
		})
	})
	When("opni cluster does exist", func() {
		var request *admin.AISettings
		BeforeEach(func() {
			request = &admin.AISettings{
				Controlplane: &admin.PretrainedModel{},
			}
		})
		Specify("setup", func() {
			_, err := plugin.PutAISettings(context.Background(), request)
			Expect(err).NotTo(HaveOccurred())
			cluster := &aiv1beta1.OpniCluster{}
			Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      OpniServicesName,
				Namespace: namespace,
			}, cluster)).Should(Succeed())
			Expect(len(cluster.Spec.Services.Inference.PretrainedModels)).To(BeNumerically("==", 1))
			list := &aiv1beta1.PretrainedModelList{}
			Expect(k8sClient.List(context.Background(), list, client.InNamespace(namespace))).To(Succeed())
			Expect(len(list.Items)).To(BeNumerically("==", 1))
		})
		When("no new version is available", func() {
			Specify("upgrade available should return false", func() {
				resp, err := plugin.UpgradeAvailable(context.Background(), &emptypb.Empty{})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.UpgradePending).NotTo(BeNil())
				Expect(*resp.UpgradePending).To(BeFalse())
			})
		})
		When("new version is available", func() {
			BeforeEach(func() {
				version = "v0.11.0"
			})
			When("updating the opni cluster", func() {
				BeforeEach(func() {
					request.Rancher = &admin.PretrainedModel{}
				})
				It("should succeed", func() {
					_, err := plugin.PutAISettings(context.Background(), request)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should update the opni cluster but not the version", func() {
					cluster := &aiv1beta1.OpniCluster{}
					Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      OpniServicesName,
						Namespace: namespace,
					}, cluster)).Should(Succeed())
					Expect(len(cluster.Spec.Services.Inference.PretrainedModels)).To(BeNumerically("==", 2))
					Expect(cluster.Spec.Version).To(Equal("v0.7.0"))
				})
				It("should create an additional pretrained model", func() {
					list := &aiv1beta1.PretrainedModelList{}
					Expect(k8sClient.List(context.Background(), list, client.InNamespace(namespace))).To(Succeed())
					Expect(len(list.Items)).To(BeNumerically("==", 2))
				})
			})
			Specify("upgrade available should return true", func() {
				resp, err := plugin.UpgradeAvailable(context.Background(), &emptypb.Empty{})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.UpgradePending).NotTo(BeNil())
				Expect(*resp.UpgradePending).To(BeTrue())
			})
			Specify("upgrade should update the cluster", func() {
				_, err := plugin.DoUpgrade(context.Background(), &emptypb.Empty{})
				Expect(err).NotTo(HaveOccurred())
				cluster := &aiv1beta1.OpniCluster{}
				Eventually(k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      OpniServicesName,
					Namespace: namespace,
				}, cluster)).Should(Succeed())
				Expect(cluster.Spec.Version).To(Equal("v0.11.0"))
			})
		})
		When("new version is older", func() {
			BeforeEach(func() {
				version = "0.6.0"
			})
			Specify("upgrade available should be false", func() {
				resp, err := plugin.UpgradeAvailable(context.Background(), &emptypb.Empty{})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.UpgradePending).NotTo(BeNil())
				Expect(*resp.UpgradePending).To(BeFalse())
			})
			Specify("upgrade should error", func() {
				_, err := plugin.DoUpgrade(context.Background(), &emptypb.Empty{})
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
