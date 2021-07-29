package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
)

func makeTestNamespace() string {
	for i := 0; i < 100; i++ {
		ns := fmt.Sprintf("test-%d", i)
		if err := k8sClient.Create(
			context.Background(),
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ns,
				},
			},
		); err != nil {
			continue
		}
		return ns
	}
	panic("could not create namespace")
}

type opniClusterOpts struct {
	Name      string
	Namespace string
	Models    []string
}

func makeOpniCluster(opts opniClusterOpts) *v1beta1.OpniCluster {
	imageSpec := v1beta1.ImageSpec{
		ImagePullPolicy: (*corev1.PullPolicy)(pointer.String(string(corev1.PullNever))),
		ImagePullSecrets: []corev1.LocalObjectReference{
			{
				Name: "lorem-ipsum",
			},
		},
	}
	return &v1beta1.OpniCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1beta1.GroupVersion.String(),
			Kind:       "OpniCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: opts.Name,
			Namespace: func() string {
				if opts.Namespace == "" {
					return makeTestNamespace()
				}
				return opts.Namespace
			}(),
		},
		Spec: v1beta1.OpniClusterSpec{
			Version:     "test",
			DefaultRepo: pointer.String("docker.biz/rancher"), // nonexistent repo
			Nats: v1beta1.NatsSpec{
				AuthMethod: v1beta1.NatsAuthUsername,
			},
			Services: v1beta1.ServicesSpec{
				Inference: v1beta1.InferenceServiceSpec{
					ImageSpec: imageSpec,
					PretrainedModels: func() []v1beta1.PretrainedModelReference {
						var ret []v1beta1.PretrainedModelReference
						for _, model := range opts.Models {
							ret = append(ret, v1beta1.PretrainedModelReference{
								Name: model,
							})
						}
						return ret
					}(),
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
}

var _ = Describe("OpniCluster Controller", func() {
	var (
		cluster                    *v1beta1.OpniCluster
		err                        error
		clusterWithPretrainedModel *v1beta1.OpniCluster
		pretrainedModelNS          string
	)
	BeforeEach(func() {
		cluster = makeOpniCluster(opniClusterOpts{
			Name:      "test",
			Namespace: "opnicluster-test",
		})
	})
	JustBeforeEach(func() {
		err = k8sClient.Create(context.Background(), cluster)
	})
	AfterEach(func() {
		namespace := "opnicluster-test"
		deletionPolicy := metav1.DeletePropagationBackground
		deployment := appsv1.Deployment{}
		statefulset := appsv1.StatefulSet{}
		secret := corev1.Secret{}
		service := corev1.Service{}
		deployments := appsv1.DeploymentList{}
		statefulsets := appsv1.StatefulSetList{}
		secrets := corev1.SecretList{}
		services := corev1.ServiceList{}
		k8sClient.DeleteAllOf(context.Background(), &deployment, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				Namespace: namespace,
			},
			DeleteOptions: client.DeleteOptions{
				GracePeriodSeconds: pointer.Int64(0),
			},
		})
		k8sClient.DeleteAllOf(context.Background(), &statefulset, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				Namespace: namespace,
			},
			DeleteOptions: client.DeleteOptions{
				GracePeriodSeconds: pointer.Int64(0),
			},
		})
		k8sClient.DeleteAllOf(context.Background(), &secret, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				Namespace: namespace,
			},
			DeleteOptions: client.DeleteOptions{
				GracePeriodSeconds: pointer.Int64(0),
			},
		})
		k8sClient.DeleteAllOf(context.Background(), &service, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				Namespace: namespace,
			},
			DeleteOptions: client.DeleteOptions{
				GracePeriodSeconds: pointer.Int64(0),
				PropagationPolicy:  &deletionPolicy,
			},
		})
		// Wait for all the things to be deleted
		Eventually(func() []appsv1.Deployment {
			k8sClient.List(context.Background(), &deployments, &client.ListOptions{
				Namespace: namespace,
			})
			return deployments.Items
		}).Should(BeEmpty())
		Eventually(func() []appsv1.StatefulSet {
			k8sClient.List(context.Background(), &statefulsets, &client.ListOptions{
				Namespace: namespace,
			})
			return statefulsets.Items
		}).Should(BeEmpty())
		Eventually(func() []corev1.Secret {
			k8sClient.List(context.Background(), &secrets, &client.ListOptions{
				Namespace: namespace,
			})
			return secrets.Items
		}).Should(BeEmpty())
		Eventually(func() []corev1.Service {
			k8sClient.List(context.Background(), &services, &client.ListOptions{
				Namespace: namespace,
			})
			return services.Items
		}).Should(BeEmpty())
	})
	JustAfterEach(func() {
		k8sClient.Delete(context.Background(), cluster, client.GracePeriodSeconds(0))
		time.Sleep(1 * time.Second)
	})
	When("creating an opnicluster ", func() {
		It("should succeed", func() {
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				}, cluster)
			}).Should(Succeed())
			Expect(cluster.TypeMeta.Kind).To(Equal("OpniCluster"))
		})
		It("should create the drain service deployment", func() {
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "drain-service",
					Namespace: cluster.Namespace,
				}, deployment)
			}).Should(Succeed())
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
					Namespace: cluster.Namespace,
				}, deployment)
			}).Should(Succeed())
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
					Namespace: cluster.Namespace,
				}, deployment)
			}).Should(Succeed())
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
					Namespace: cluster.Namespace,
				}, deployment)
			}).Should(Succeed())
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
						Namespace: cluster.Namespace,
					},
				}
				Eventually(func() error {
					return k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      deployment.Name,
						Namespace: deployment.Namespace,
					}, deployment)
				}).Should(Succeed())
				Expect(deployment.Labels).To(And(
					HaveKeyWithValue(resources.AppNameLabel, kind.ServiceName()),
					HaveKeyWithValue(resources.ServiceLabel, kind.String()),
					HaveKeyWithValue(resources.PartOfLabel, "opni"),
				))
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
						Namespace: cluster.Namespace,
					},
				}
				Eventually(func() error {
					return k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      deployment.Name,
						Namespace: deployment.Namespace,
					}, deployment)
				}).Should(Succeed())
				k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
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
				Namespace:     cluster.Namespace,
				LabelSelector: labels.NewSelector().Add(*req),
			})
			Expect(deployments.Items).To(BeEmpty())
		})
		It("should create a nats statefulset", func() {
			statefulset := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-nats", cluster.Name),
					Namespace: cluster.Namespace,
				}, statefulset)
			}).Should(Succeed())
			Expect(*statefulset.Spec.Replicas).To(Equal(int32(3)))
			k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
			Expect(statefulset).To(BeOwnedBy(cluster))
		})
		It("should create a nats config secret", func() {
			secret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-nats-config", cluster.Name),
					Namespace: cluster.Namespace,
				}, secret)
			}).Should(Succeed())
			Expect(secret.Data).To(HaveKey("nats-config.conf"))
			k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
			Expect(secret).To(BeOwnedBy(cluster))
		})
		It("should create a headless nats service", func() {
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-nats-headless", cluster.Name),
					Namespace: cluster.Namespace,
				}, service)
			}).Should(Succeed())
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.ClusterIP).To(Equal("None"))
			Expect(service.Spec.Selector).To(And(
				HaveKeyWithValue(resources.AppNameLabel, "nats"),
				HaveKeyWithValue(resources.OpniClusterName, cluster.Name),
			))
			Expect(service.Spec.Ports).To(ContainElements(
				corev1.ServicePort{
					Name:       "tcp-client",
					Port:       4222,
					TargetPort: intstr.FromString("client"),
					Protocol:   corev1.ProtocolTCP,
				},
				corev1.ServicePort{
					Name:       "tcp-cluster",
					Port:       6222,
					TargetPort: intstr.FromString("cluster"),
					Protocol:   corev1.ProtocolTCP,
				},
			))
			k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
			Expect(service).To(BeOwnedBy(cluster))
		})
		It("should create a nats cluster service", func() {
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-nats-cluster", cluster.Name),
					Namespace: cluster.Namespace,
				}, service)
			}).Should(Succeed())
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.ClusterIP).NotTo(Or(
				BeEmpty(),
				Equal("None"),
			))
			Expect(service.Spec.Selector).To(And(
				HaveKeyWithValue(resources.AppNameLabel, "nats"),
				HaveKeyWithValue(resources.OpniClusterName, cluster.Name),
			))
			Expect(service.Spec.Ports).To(ContainElement(
				corev1.ServicePort{
					Name:       "tcp-cluster",
					Port:       6222,
					TargetPort: intstr.FromString("cluster"),
					Protocol:   corev1.ProtocolTCP,
				}))
			k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
			Expect(service).To(BeOwnedBy(cluster))
		})
		It("should create a nats client service", func() {
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-nats-client", cluster.Name),
					Namespace: cluster.Namespace,
				}, service)
			}).Should(Succeed())
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.ClusterIP).NotTo(Or(
				BeEmpty(),
				Equal("None"),
			))
			Expect(service.Spec.Selector).To(And(
				HaveKeyWithValue(resources.AppNameLabel, "nats"),
				HaveKeyWithValue(resources.OpniClusterName, cluster.Name),
			))
			Expect(service.Spec.Ports).To(ContainElement(
				corev1.ServicePort{
					Name:       "tcp-client",
					Port:       4222,
					TargetPort: intstr.FromString("client"),
					Protocol:   corev1.ProtocolTCP,
				}))
			k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
			Expect(service).To(BeOwnedBy(cluster))
		})
		It("should create a nats password secret", func() {
			secret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-nats-client", cluster.Name),
					Namespace: cluster.Namespace,
				}, secret)
			}).Should(Succeed())
			Expect(secret.Data).To(HaveKey("password"))
			k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
			Expect(secret).To(BeOwnedBy(cluster))
		})
	})
	When("creating a pretrained model", func() {
		// Not testing that the pretrained model controller works here, as that
		// is tested in the pretrained model controller test.
		It("should succeed", func() {
			pretrainedModelNS = makeTestNamespace()
			Expect(k8sClient.Create(context.Background(), &v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model",
					Namespace: pretrainedModelNS,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: "https://foo.bar/model.tar.gz",
						},
					},
					Hyperparameters: hyperparameters,
				},
			})).To(Succeed())
		})
	})
	Context("providing an auth secret for nats should function", func() {
		BeforeEach(func() {
			cluster.Spec.Nats.PasswordFrom = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "test-password-secret",
				},
				Key: "password",
			}
		})
		When("auth secret doesn't exist", func() {
			It("should succeed", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("should not create nats", func() {
				Consistently(func() bool {
					// Ensure the deployment is not created
					statefulset := &appsv1.StatefulSet{}
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      fmt.Sprintf("%s-nats", cluster.Name),
						Namespace: cluster.Namespace,
					}, statefulset)
					return errors.IsNotFound(err)
				}).Should(BeTrue())
			})
		})
		When("auth secret exists but password key doesn't", func() {
			It("should not create nats", func() {
				authSecret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-password-secret",
						Namespace: cluster.Namespace,
					},
					Data: map[string][]byte{
						"secret": []byte("testing"),
					},
				}
				k8sClient.Create(context.Background(), &authSecret)
				Consistently(func() bool {
					// Ensure the deployment is not created
					statefulset := &appsv1.StatefulSet{}
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      fmt.Sprintf("%s-nats", cluster.Name),
						Namespace: cluster.Namespace,
					}, statefulset)
					return errors.IsNotFound(err)
				}).Should(BeTrue())
			})
		})
		When("auth secret exists with correct key", func() {
			It("should create nats", func() {
				authSecret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-password-secret",
						Namespace: cluster.Namespace,
					},
					Data: map[string][]byte{
						"password": []byte("testing"),
					},
				}
				k8sClient.Create(context.Background(), &authSecret)
				Eventually(func() error {
					statefulset := &appsv1.StatefulSet{}
					return k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      fmt.Sprintf("%s-nats", cluster.Name),
						Namespace: cluster.Namespace,
					}, statefulset)
				}).Should(Succeed())
			})
			It("should not create a separate auth secret", func() {
				Consistently(func() bool {
					secret := &corev1.Secret{}
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      fmt.Sprintf("%s-nats-client", cluster.Name),
						Namespace: cluster.Namespace,
					}, secret)
					return errors.IsNotFound(err)
				})
			})
		})

	})
	XWhen("using nkey auth", func() {
		BeforeEach(func() {
			cluster.Spec.Nats.AuthMethod = v1beta1.NatsAuthNkey
		})
		It("should create nats", func() {
			Eventually(func() error {
				statefulset := &appsv1.StatefulSet{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-nats", cluster.Name),
					Namespace: cluster.Namespace,
				}, statefulset)
			}).Should(Succeed())
		})
		It("should not create an auth secret", func() {
			secret := &corev1.Secret{}
			Consistently(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-nats-client", cluster.Name),
					Namespace: cluster.Namespace,
				}, secret)
				return errors.IsNotFound(err)
			})
			Expect(secret.Data).To(HaveKey("seed"))
		})
		It("should update the cluster status", func() {
			k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
			Expect(cluster.Status.Auth.NKeyUser).NotTo(BeEmpty())
		})
	})
	When("referencing the pretrained model in an opnicluster", func() {
		It("should succeed", func() {
			clusterWithPretrainedModel = makeOpniCluster(opniClusterOpts{
				Name:      "test-cluster",
				Namespace: pretrainedModelNS,
				Models:    []string{"test-model"},
			})
			Expect(k8sClient.Create(context.Background(), clusterWithPretrainedModel)).To(Succeed())
		})
		It("should create an inference service for the pretrained model", func() {
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "inference-service-test-model",
					Namespace: clusterWithPretrainedModel.Namespace,
				}, deployment)
			}).Should(Succeed())
			Expect(deployment.Spec.Template.Spec.InitContainers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.InitContainers[0].Image).
				To(Equal("docker.io/curlimages/curl:latest"))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
		})
	})
	When("deleting an opnicluster", func() {
		It("should succeed", func() {
			Expect(k8sClient.Delete(context.Background(), clusterWithPretrainedModel)).To(Succeed())
		})
		It("should delete the pretrained model inference service", func() {
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "inference-service-test-model",
					Namespace: clusterWithPretrainedModel.Namespace,
				}, deployment)
			}).Should(HaveOccurred())
		})
	})

	Context("pretrained models should function in various configurations", func() {
		It("should ignore duplicate model names", func() {
			cluster := makeOpniCluster(opniClusterOpts{
				Name: "test-model",
				Models: []string{
					"test-model",
					"test-model",
				},
			})
			Expect(k8sClient.Create(context.Background(), &v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model",
					Namespace: cluster.Namespace,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: "https://foo.bar/model.tar.gz",
						},
					},
					Hyperparameters: hyperparameters,
				},
			})).To(Succeed())
			// create cluster with 2 copies of the same model
			Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())

			// check that the second instance is ignored
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.In, []string{"test-model"})
			Expect(err).NotTo(HaveOccurred())
			deployments := &appsv1.DeploymentList{}
			Eventually(func() int {
				k8sClient.List(context.Background(), deployments, &client.ListOptions{
					Namespace:     cluster.Namespace,
					LabelSelector: labels.NewSelector().Add(*req),
				})
				return len(deployments.Items)
			}).Should(Equal(1))
		})
		It("should work with multiple different models", func() {
			ns := makeTestNamespace()
			// Create 2 different pretrained models
			model1 := v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-1",
					Namespace: ns,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: "https://foo.bar/model.tar.gz",
						},
					},
					Hyperparameters: []v1beta1.Hyperparameter{
						{
							Name:  "foo",
							Value: "0.1",
						},
					},
				},
			}
			model2 := v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-2",
					Namespace: ns,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: "https://bar.baz/model.tar.gz",
						},
					},
					Hyperparameters: []v1beta1.Hyperparameter{
						{
							Name:  "bar",
							Value: "0.2",
						},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &model1)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), &model2)).To(Succeed())
			// Create cluster with both models
			cluster := makeOpniCluster(opniClusterOpts{
				Name:      "test-model-3",
				Namespace: ns,
				Models: []string{
					"test-model-1",
					"test-model-2",
				},
			})
			Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())
			// check that the two different models are created
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.In, []string{
					"test-model-1",
					"test-model-2",
				})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() int {
				deployments := &appsv1.DeploymentList{}
				k8sClient.List(context.Background(), deployments, &client.ListOptions{
					Namespace:     ns,
					LabelSelector: labels.NewSelector().Add(*req),
				})
				return len(deployments.Items)
			}).Should(Equal(2))
		})
		It("should work with models with different source configurations", func() {
			ns := makeTestNamespace()
			// Create 2 different pretrained models
			model1 := v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-1",
					Namespace: ns,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: "https://foo.bar/model.tar.gz",
						},
					},
					Hyperparameters: []v1beta1.Hyperparameter{
						{
							Name:  "foo",
							Value: "0.1",
						},
					},
				},
			}
			model2 := v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-2",
					Namespace: ns,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						Container: &v1beta1.ContainerSource{
							Image: "gcr.io/foo/bar:latest",
						},
					},
					Hyperparameters: []v1beta1.Hyperparameter{
						{
							Name:  "baz",
							Value: "0.3",
						},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &model1)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), &model2)).To(Succeed())
			// Create cluster with both models
			cluster := makeOpniCluster(opniClusterOpts{
				Name:      "test-cluster",
				Namespace: ns,
				Models: []string{
					"test-model-1",
					"test-model-2",
				},
			})
			Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())
			// check that the two different models are created
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.In, []string{
					"test-model-1",
					"test-model-2",
				})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() int {
				deployments := &appsv1.DeploymentList{}
				k8sClient.List(context.Background(), deployments, &client.ListOptions{
					Namespace:     ns,
					LabelSelector: labels.NewSelector().Add(*req),
				})
				return len(deployments.Items)
			}).Should(Equal(2))
		})
	})
	var namespaceFromPreviousTest string
	When("adding pretrained models to an existing opnicluster", func() {
		It("should reconcile the pretrained model deployments", func() {
			namespaceFromPreviousTest = makeTestNamespace() // this will make sense later
			By("adding an opnicluster without any models")
			cluster := makeOpniCluster(opniClusterOpts{
				Name:      "test-cluster",
				Namespace: namespaceFromPreviousTest,
			})
			model := v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model",
					Namespace: cluster.Namespace,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: "https://foo.bar/model.tar.gz",
						},
					},
					Hyperparameters: []v1beta1.Hyperparameter{
						{
							Name:  "foo",
							Value: "0.1",
						},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())
			By("creating a model")
			Expect(k8sClient.Create(context.Background(), &model)).To(Succeed())
			By("adding the model to the opnicluster")
			// get the latest updates to the object
			k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
			cluster.Spec.Services.Inference.PretrainedModels =
				append(cluster.Spec.Services.Inference.PretrainedModels,
					v1beta1.PretrainedModelReference{
						Name: model.Name,
					},
				)
			Expect(k8sClient.Update(context.Background(), cluster)).To(Succeed())

			By("verifying the pretrained model deployment is created")
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.In, []string{
					model.Name,
				})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() int {
				deployments := &appsv1.DeploymentList{}
				k8sClient.List(context.Background(), deployments, &client.ListOptions{
					Namespace:     cluster.Namespace,
					LabelSelector: labels.NewSelector().Add(*req),
				})
				return len(deployments.Items)
			}).Should(Equal(1))
		})
	})
	When("deleting a pretrained model from an existing opnicluster", func() {
		It("should reconcile the pretrained model deployments", func() {
			By("creating a model")
			ns := makeTestNamespace()
			model := v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model",
					Namespace: ns,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: "https://foo.bar/model.tar.gz",
						},
					},
					Hyperparameters: []v1beta1.Hyperparameter{
						{
							Name:  "foo",
							Value: "0.1",
						},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &model)).To(Succeed())
			By("creating an opnicluster with the model")
			cluster := makeOpniCluster(opniClusterOpts{
				Name:      "test-cluster",
				Namespace: ns,
			})
			cluster.Spec.Services.Inference.PretrainedModels =
				append(cluster.Spec.Services.Inference.PretrainedModels,
					v1beta1.PretrainedModelReference{
						Name: model.Name,
					},
				)
			Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())
			By("verifying the model is added to the opnicluster")
			k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
			Expect(len(cluster.Spec.Services.Inference.PretrainedModels)).To(Equal(1))
			Expect(cluster.Spec.Services.Inference.PretrainedModels[0].Name).To(Equal(model.Name))
			By("waiting for the model deployment to be created")
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.In, []string{
					model.Name,
				})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() int {
				deployments := &appsv1.DeploymentList{}
				k8sClient.List(context.Background(), deployments, &client.ListOptions{
					Namespace:     cluster.Namespace,
					LabelSelector: labels.NewSelector().Add(*req),
				})
				return len(deployments.Items)
			}).Should(Equal(1))
			By("deleting the model from the opnicluster")
			k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)
			cluster.Spec.Services.Inference.PretrainedModels =
				cluster.Spec.Services.Inference.PretrainedModels[:0]
			Expect(k8sClient.Update(context.Background(), cluster)).To(Succeed())
			By("verifying the model deployment is deleted")
			Eventually(func() int {
				deployments := &appsv1.DeploymentList{}
				k8sClient.List(context.Background(), deployments, &client.ListOptions{
					Namespace:     cluster.Namespace,
					LabelSelector: labels.NewSelector().Add(*req),
				})
				return len(deployments.Items)
			}).Should(Equal(0))
		})
	})
	When("deleting an opnicluster with a pretrained model", func() {
		It("should succeed", func() {
			By("deleting an opnicluster with a pretrained model")
			// Look up the opnicluster from the previous test
			cluster := &v1beta1.OpniCluster{}
			k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      "test-cluster",
				Namespace: namespaceFromPreviousTest,
			}, cluster)
			Expect(k8sClient.Delete(context.Background(), cluster)).To(Succeed())
			// we can't actually delete things here so we can just test for
			// proper object ownership

			// Look up the matching pretrainedmodel
			model := &v1beta1.PretrainedModel{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      "test-model",
				Namespace: namespaceFromPreviousTest,
			}, model)
			Expect(err).NotTo(HaveOccurred())
			Expect(model).NotTo(BeOwnedBy(cluster))

			// Look up the matching deployment by label
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.In, []string{
					model.Name,
				})
			Expect(err).NotTo(HaveOccurred())
			deployments := &appsv1.DeploymentList{}
			k8sClient.List(context.Background(), deployments, &client.ListOptions{
				Namespace:     cluster.Namespace,
				LabelSelector: labels.NewSelector().Add(*req),
			})
			Expect(len(deployments.Items)).To(Equal(1))
			// the deployment should be owned by the cluster, not the model
			Expect(&deployments.Items[0]).To(BeOwnedBy(cluster))
			Expect(&deployments.Items[0]).NotTo(BeOwnedBy(model))
			Expect(model).NotTo(BeOwnedBy(cluster))
			Expect(cluster).NotTo(BeOwnedBy(model))
		})
	})
	When("creating an opnicluster with an invalid pretrained model", func() {
		var ns string
		It("should wait and have a status condition", func() {
			By("creating an opnicluster with an invalid pretrained model")
			cluster := makeOpniCluster(opniClusterOpts{
				Name:   "test-cluster",
				Models: []string{"invalid-model"},
			})
			ns = cluster.Namespace
			Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())
			By("verifying the model deployment is in the waiting state")
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.In, []string{
					"invalid-model",
				})
			Expect(err).NotTo(HaveOccurred())
			done := make(chan struct{})
			go func() {
				defer GinkgoRecover()
				defer close(done)
				Consistently(func() string {
					// Ensure the deployment is not created
					deployments := &appsv1.DeploymentList{}
					k8sClient.List(context.Background(), deployments, &client.ListOptions{
						Namespace:     cluster.Namespace,
						LabelSelector: labels.NewSelector().Add(*req),
					})
					if len(deployments.Items) != 0 {
						return "Expected pretrained model deployment not to be created"
					}
					return ""
				}).Should(Equal(""))
			}()
			// Ensure the state is set to error
			Eventually(func() v1beta1.OpniClusterState {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "test-cluster",
					Namespace: ns,
				}, cluster)
				if err != nil {
					return ""
				}
				return cluster.Status.State
			}).Should(Equal(v1beta1.OpniClusterStateError))
			Expect(cluster.Status.Conditions).NotTo(BeEmpty())
			<-done
		})
		It("should resolve when the pretrained model is created", func() {
			// Create the invalid model
			model := v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-model",
					Namespace: ns,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: "http://invalid-model",
						},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &model)).To(Succeed())
			cluster := &v1beta1.OpniCluster{}
			// Wait for the cluster to be ready
			Eventually(func() v1beta1.OpniClusterState {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "test-cluster",
					Namespace: ns,
				}, cluster)
				Expect(err).NotTo(HaveOccurred())
				return cluster.Status.State
			}).ShouldNot(Equal(v1beta1.OpniClusterStateError))
		})
	})
	// TODO: decide how to handle deleting pretrainedmodels in use
	PWhen("deleting a pretrained model while an opnicluster is using it", func() {
		PIt("should succeed", func() {})
		PIt("should delete the inference service", func() {})
		PIt("should delete the pretrainedmodel resource", func() {})
		PIt("should cause the opnicluster to report a status condition", func() {})
	})
})
