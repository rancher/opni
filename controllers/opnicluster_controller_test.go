package controllers

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
)

var _ = FDescribe("OpniCluster Controller", func() {
	imageSpec := v1beta1.ImageSpec{
		ImagePullPolicy: (*corev1.PullPolicy)(pointer.String(string(corev1.PullNever))),
		ImagePullSecrets: []corev1.LocalObjectReference{
			{
				Name: "lorem-ipsum",
			},
		},
	}
	cluster := &v1beta1.OpniCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1beta1.GroupVersion.String(),
			Kind:       "OpniCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opnicluster-controller-test1",
			Namespace: crNamespace,
		},
		Spec: v1beta1.OpniClusterSpec{
			Version:     "test",
			DefaultRepo: pointer.String("docker.biz/rancher"), // nonexistent repo
			Services: v1beta1.ServicesSpec{
				Inference: v1beta1.InferenceServiceSpec{
					ImageSpec: imageSpec,
				},
				Drain: v1beta1.DrainServiceSpec{
					ImageSpec: imageSpec,
				},
				Preprocessing: v1beta1.PreprocessingServiceSpec{
					ImageSpec: imageSpec,
				},
				PayloadReceiver: v1beta1.PayloadReceiverServiceSpec{
					ImageSpec: v1beta1.ImageSpec{
						Image: pointer.String("foo"),
					},
				},
			},
		},
	}
	When("creating an opnicluster ", func() {
		It("should succeed", func() {
			err := k8sClient.Create(context.Background(), cluster)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      cluster.Name,
					Namespace: crNamespace,
				}, cluster)
			}, timeout, interval).Should(Succeed())
			Expect(cluster.TypeMeta.Kind).To(Equal("OpniCluster"))
		})
		It("should create the drain service deployment", func() {
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "drain-service",
					Namespace: crNamespace,
				}, deployment)
			}, timeout, interval).Should(Succeed())
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(
				Equal("docker.biz/rancher/opni-drain-service:test"))
			Expect(deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(
				Equal(corev1.PullNever))
			Expect(deployment.Spec.Template.Spec.ImagePullSecrets[0].Name).To(
				Equal("lorem-ipsum"))
		})
		It("should create the inference service deployment", func() {
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "inference-service",
					Namespace: crNamespace,
				}, deployment)
			}, timeout, interval).Should(Succeed())
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(
				Equal("docker.biz/rancher/opni-inference-service:test"))
			Expect(deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(
				Equal(corev1.PullNever))
			Expect(deployment.Spec.Template.Spec.ImagePullSecrets[0].Name).To(
				Equal("lorem-ipsum"))
		})
		It("should create the preprocessing service deployment", func() {
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "preprocessing-service",
					Namespace: crNamespace,
				}, deployment)
			}, timeout, interval).Should(Succeed())
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(
				Equal("docker.biz/rancher/opni-preprocessing-service:test"))
			Expect(deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(
				Equal(corev1.PullNever))
			Expect(deployment.Spec.Template.Spec.ImagePullSecrets[0].Name).To(
				Equal("lorem-ipsum"))
		})
		It("should create the payload receiver service deployment", func() {
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "payload-receiver-service",
					Namespace: crNamespace,
				}, deployment)
			}, timeout, interval).Should(Succeed())
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("foo"))
		})
		It("should apply the correct labels to service pods", func() {
			for _, kind := range []v1beta1.ServiceKind{
				v1beta1.DrainService,
				v1beta1.InferenceService,
				v1beta1.PayloadReceiverService,
				v1beta1.PreprocessingService,
			} {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kind.ServiceName(),
						Namespace: crNamespace,
					},
				}
				Eventually(func() error {
					return k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      deployment.Name,
						Namespace: deployment.Namespace,
					}, deployment)
				}, timeout, interval).Should(Succeed())
				Expect(deployment.Spec.Template.Labels).To(And(
					HaveKeyWithValue(resources.AppNameLabel, kind.ServiceName()),
					HaveKeyWithValue(resources.ServiceLabel, kind.String()),
					HaveKeyWithValue(resources.PartOfLabel, "opni"),
				))
			}
		})
		It("should set the owner reference for each service to the opnicluster", func() {
			for _, kind := range []v1beta1.ServiceKind{
				v1beta1.DrainService,
				v1beta1.InferenceService,
				v1beta1.PayloadReceiverService,
				v1beta1.PreprocessingService,
			} {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kind.ServiceName(),
						Namespace: crNamespace,
					},
				}
				Eventually(func() error {
					return k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      deployment.Name,
						Namespace: deployment.Namespace,
					}, deployment)
				}, timeout, interval).Should(Succeed())
				Expect(deployment).To(BeOwnedBy(cluster))
			}
		})
		It("should not create any pretrained model services yet", func() {
			// Identify pretrained model services with the label "opni.io/pretrained-model"
			deployments := &appsv1.DeploymentList{}
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.Exists, nil)
			Expect(err).NotTo(HaveOccurred())
			k8sClient.List(context.Background(), deployments, &client.ListOptions{
				LabelSelector: labels.NewSelector().Add(*req),
			})
			Expect(deployments.Items).To(BeEmpty())
		})
	})
	When("creating a pretrained model", func() {
		// Not testing that the pretrained model controller works here, as that
		// is tested in the pretrained model controller test.
		It("should succeed", func() {

		})
	})
	When("referencing the pretrained model in an opnicluster", func() {
		It("should succeed", func() {})
		It("should create an inference service for the pretrained model", func() {})
	})
	Context("pretrained models should function in various configurations", func() {
		It("should work with multiple copies of a model specified", func() {})
		It("should work with multiple different models", func() {})
		It("should work with models with different source configurations", func() {})
	})
	When("adding pretrained models to an existing opnicluster", func() {
		It("should succeed", func() {})
		It("should create an inference service for the pretrained model", func() {})
	})
	When("deleting a pretrained model from an existing opnicluster", func() {
		It("should succeed", func() {})
		It("should delete the inference service", func() {})
	})
	When("deleting an opnicluster with a pretrained model", func() {
		It("should succeed", func() {})
		It("should delete the inference service", func() {})
		It("should keep the pretrainedmodel resource", func() {})
	})
	When("creating an opnicluster with an invalid pretrained model", func() {
		It("should wait and have a status condition", func() {})
		It("should resolve when the pretrained model is created", func() {})
	})
	When("deleting a pretrained model while an opnicluster is using it", func() {
		It("should succeed", func() {})
		It("should delete the inference service", func() {})
		It("should delete the pretrainedmodel resource", func() {})
		It("should cause the opnicluster to report a status condition", func() {})
	})
	When("deleting an opnicluster with a model that is also being used by another opnicluster", func() {
		It("should succeed", func() {})
		It("should delete the inference service only for the deleted opnicluster", func() {})
		It("should not delete the pretrainedmodel resource", func() {})
		It("should not cause the remaining opnicluster to report a status condition", func() {})
	})
})
