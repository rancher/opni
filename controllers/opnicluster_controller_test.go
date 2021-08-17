package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/wait"
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

func updateObject(existing client.Object, patchFn interface{}) {
	patchFnValue := reflect.ValueOf(patchFn)
	if patchFnValue.Kind() != reflect.Func {
		panic("patchFn must be a function")
	}
	var lastErr error
	waitErr := wait.ExponentialBackoff(wait.Backoff{
		Duration: 10 * time.Millisecond,
		Factor:   2,
		Steps:    10,
	}, func() (bool, error) {
		// Make a copy of the existing object
		existingCopy := existing.DeepCopyObject().(client.Object)
		// Get the latest version of the object
		lastErr = k8sClient.Get(context.Background(),
			client.ObjectKeyFromObject(existingCopy), existingCopy)
		if lastErr != nil {
			return false, nil
		}
		// Call the patchFn to make changes to the object
		patchFnValue.Call([]reflect.Value{reflect.ValueOf(existingCopy)})
		// Apply the patch
		lastErr = k8sClient.Update(context.Background(), existingCopy, &client.UpdateOptions{})
		if lastErr != nil {
			return false, nil
		}
		// Replace the existing object with the new one
		existing = existingCopy
		return true, nil // exit backoff loop
	})
	if waitErr != nil {
		Fail("failed to update object: " + lastErr.Error())
	}
}

type opniClusterOpts struct {
	Name                string
	Namespace           string
	Models              []string
	DisableOpniServices bool
}

func buildCluster(opts opniClusterOpts) *v1beta1.OpniCluster {
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
					Enabled:   pointer.Bool(!opts.DisableOpniServices),
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
					Enabled:   pointer.Bool(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				Preprocessing: v1beta1.PreprocessingServiceSpec{
					Enabled:   pointer.Bool(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				PayloadReceiver: v1beta1.PayloadReceiverServiceSpec{
					Enabled:   pointer.Bool(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				GPUController: v1beta1.GPUControllerServiceSpec{
					Enabled:   pointer.Bool(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
			},
		},
	}
}

var _ = Describe("OpniCluster Controller", func() {
	cluster := &v1beta1.OpniCluster{}

	createCluster := func(c *v1beta1.OpniCluster) {
		err := k8sClient.Create(context.Background(), c)
		Expect(err).NotTo(HaveOccurred())
		Eventually(Object(c)).Should(Exist())
		Expect(k8sClient.Get(
			context.Background(),
			client.ObjectKeyFromObject(c),
			cluster,
		)).To(Succeed())
	}

	It("should create the necessary opni service deployments", func() {
		By("waiting for the cluster to be created")
		createCluster(buildCluster(opniClusterOpts{Name: "test"}))

		for _, kind := range []v1beta1.ServiceKind{
			v1beta1.DrainService,
			v1beta1.InferenceService,
			v1beta1.PayloadReceiverService,
			v1beta1.PreprocessingService,
			v1beta1.GPUControllerService,
		} {
			By(fmt.Sprintf("checking %s service metadata", kind.String()))
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kind.ServiceName(),
					Namespace: cluster.Namespace,
				},
			})).Should(ExistAnd(
				HaveLabels(
					resources.AppNameLabel, kind.ServiceName(),
					resources.ServiceLabel, kind.String(),
					resources.PartOfLabel, "opni",
				),
				HaveOwner(cluster),
			))
			By(fmt.Sprintf("checking %s service containers", kind.String()))
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kind.ServiceName(),
					Namespace: cluster.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(
					HaveImage(fmt.Sprintf("docker.biz/rancher/%s:test", kind.ImageName()), corev1.PullNever),
				),
				HaveImagePullSecrets("lorem-ipsum"),
			))
		}
		By("checking that pretrained model services are not created yet")
		// Identify pretrained model services with the label "opni.io/pretrained-model"
		req, err := labels.NewRequirement(
			resources.PretrainedModelLabel, selection.Exists, nil)
		Expect(err).NotTo(HaveOccurred())
		Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
			Namespace:     cluster.Namespace,
			LabelSelector: labels.NewSelector().Add(*req),
		})).Should(BeEmpty())

		By("checking nats statefulset")
		Eventually(Object(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-nats",
				Namespace: cluster.Namespace,
			},
		})).Should(HaveReplicaCount(3))

		By("checking nats config secret")
		Eventually(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-config", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveData("nats-config.conf", nil),
			HaveOwner(cluster),
		))

		By("checking nats headless service")
		Eventually(Object(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-headless", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveLabels(
				"app.kubernetes.io/name", "nats",
				"app.kubernetes.io/part-of", "opni",
				"opni.io/cluster-name", cluster.Name,
			),
			BeHeadless(),
			HaveOwner(cluster),
			HavePorts("tcp-client", "tcp-cluster"),
		))

		By("checking nats cluster service")
		Eventually(Object(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-cluster", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveLabels(
				"app.kubernetes.io/name", "nats",
				"app.kubernetes.io/part-of", "opni",
				"opni.io/cluster-name", cluster.Name,
			),
			HavePorts("tcp-cluster"),
			HaveType(corev1.ServiceTypeClusterIP),
			Not(BeHeadless()),
		))

		By("checking nats client service")
		Eventually(Object(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-client", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveLabels(
				"app.kubernetes.io/name", "nats",
				"app.kubernetes.io/part-of", "opni",
				"opni.io/cluster-name", cluster.Name,
			),
			HavePorts("tcp-client"),
			HaveType(corev1.ServiceTypeClusterIP),
			Not(BeHeadless()),
		))

		By("checking nats password secret")
		Eventually(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-client", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveData("password", nil),
			HaveOwner(cluster),
		))
	})
	It("should create inference services for pretrained models", func() {
		ns := makeTestNamespace()
		// Not testing that the pretrained model controller works here, as that
		// is tested in the pretrained model controller test.
		By("creating a pretrained model")
		Expect(k8sClient.Create(context.Background(), &v1beta1.PretrainedModel{
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
			},
		})).To(Succeed())

		By("creating a cluster")
		createCluster(buildCluster(opniClusterOpts{
			Name:      "test-cluster",
			Namespace: ns,
			Models:    []string{"test-model"},
		}))

		By("checking if an inference service is created")
		Eventually(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-inference-test-model",
				Namespace: ns,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveLabels("opni.io/pretrained-model", "test-model"),
			HaveMatchingInitContainer(HaveImage("docker.io/curlimages/curl:latest")),
			HaveMatchingContainer(HaveName("inference-service")),
			HaveMatchingVolume(HaveName("model-volume")),
		))

		By("deleting the cluster")
		Expect(k8sClient.Delete(context.Background(), cluster)).To(Succeed())

		By("checking if the inference service is deleted")
		Eventually(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-inference-test-model",
				Namespace: ns,
			},
		})).ShouldNot(Exist())
	})
	Specify("providing an auth secret for nats should function", func() {
		By("creating an opnicluster")
		c := buildCluster(opniClusterOpts{
			Name: "test-cluster",
		})
		c.Spec.Nats.PasswordFrom = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "test-password-secret",
			},
			Key: "password",
		}
		createCluster(c)

		By("checking that the auth secret does not exist")
		Consistently(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Spec.Nats.PasswordFrom.Name,
				Namespace: cluster.Namespace,
			},
		})).ShouldNot(Exist())

		By("checking that the nats cluster does not exist")
		Consistently(Object(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-nats",
				Namespace: cluster.Namespace,
			},
		})).ShouldNot(Exist())

		By("creating the missing secret with the wrong key")
		Expect(k8sClient.Create(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Spec.Nats.PasswordFrom.Name,
				Namespace: cluster.Namespace,
			},
			StringData: map[string]string{
				"wrong": "wrong",
			},
		})).To(Succeed())

		By("checking that the nats cluster still does not exist")
		Consistently(Object(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-nats",
				Namespace: cluster.Namespace,
			},
		})).ShouldNot(Exist())

		By("creating the missing secret with the correct key")
		Expect(k8sClient.Delete(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Spec.Nats.PasswordFrom.Name,
				Namespace: cluster.Namespace,
			},
		})).To(Succeed())
		Expect(k8sClient.Create(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Spec.Nats.PasswordFrom.Name,
				Namespace: cluster.Namespace,
			},
			StringData: map[string]string{
				"password": "test-password",
			},
		})).To(Succeed())

		By("checking that nats is created")
		Eventually(Object(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-nats",
				Namespace: cluster.Namespace,
			},
		})).Should(Exist())

		By("ensuring a separate auth secret is not created")
		Consistently(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-client", cluster.Name),
				Namespace: cluster.Namespace,
			},
		}).ShouldNot(Exist())
	})

	Specify("nats should work using nkey auth", func() {
		c := buildCluster(opniClusterOpts{
			Name: "test-cluster",
		})
		c.Spec.Nats.AuthMethod = v1beta1.NatsAuthNkey

		By("creating an opnicluster")
		createCluster(c)

		By("checking that nats is created")
		Eventually(Object(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-nats",
				Namespace: cluster.Namespace,
			},
		})).Should(Exist())

		By("checking that an auth secret was created")
		Eventually(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-client", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveData("seed", nil),
		))

		By("checking if cluster status is updated")
		Eventually(Object(cluster)).Should(
			MatchStatus(func(s v1beta1.OpniClusterStatus) bool {
				return s.Auth.NKeyUser != "" &&
					s.Auth.AuthSecretKeyRef != nil &&
					s.Auth.AuthSecretKeyRef.Name == fmt.Sprintf("%s-nats-client", cluster.Name)
			}),
		)
	})

	Context("pretrained models should function in various configurations", func() {
		It("should ignore duplicate model names", func() {
			createCluster(buildCluster(opniClusterOpts{
				Name: "test",
				Models: []string{
					"test-model",
					"test-model",
				},
			}))
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
				},
			})).To(Succeed())

			// check that the second instance is ignored
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.In, []string{"test-model"})
			Expect(err).NotTo(HaveOccurred())
			Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
				LabelSelector: labels.NewSelector().Add(*req),
				Namespace:     cluster.Namespace,
			})).Should(HaveLen(1))
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
			createCluster(buildCluster(opniClusterOpts{
				Name:      "test-model-3",
				Namespace: ns,
				Models: []string{
					"test-model-1",
					"test-model-2",
				},
			}))

			// check that the two different models are created
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.In, []string{
					"test-model-1",
					"test-model-2",
				})
			Expect(err).NotTo(HaveOccurred())
			Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
				LabelSelector: labels.NewSelector().Add(*req),
				Namespace:     ns,
			})).Should(HaveLen(2))
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
			createCluster(buildCluster(opniClusterOpts{
				Name:      "test-cluster",
				Namespace: ns,
				Models: []string{
					"test-model-1",
					"test-model-2",
				},
			}))

			// check that the two different models are created
			req, err := labels.NewRequirement(
				resources.PretrainedModelLabel, selection.In, []string{
					"test-model-1",
					"test-model-2",
				})
			Expect(err).NotTo(HaveOccurred())
			Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
				LabelSelector: labels.NewSelector().Add(*req),
				Namespace:     ns,
			})).Should(HaveLen(2))
		})
	})
	It("should allow adding pretrained models to an existing opnicluster", func() {
		By("creating an opnicluster without any models")
		createCluster(buildCluster(opniClusterOpts{
			Name: "test-cluster",
		}))

		By("creating a model")
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
		Expect(k8sClient.Create(context.Background(), &model)).To(Succeed())

		By("adding the model to the opnicluster")
		updateObject(cluster, func(c *v1beta1.OpniCluster) {
			c.Spec.Services.Inference.PretrainedModels =
				append(c.Spec.Services.Inference.PretrainedModels,
					v1beta1.PretrainedModelReference{
						Name: model.Name,
					})
		})

		By("verifying the pretrained model deployment is created")
		req, err := labels.NewRequirement(
			resources.PretrainedModelLabel, selection.In, []string{
				model.Name,
			})
		Expect(err).NotTo(HaveOccurred())
		Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
			LabelSelector: labels.NewSelector().Add(*req),
			Namespace:     cluster.Namespace,
		})).Should(HaveLen(1))
	})
	It("should handle editing models in an existing opnicluster", func() {
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
		c := buildCluster(opniClusterOpts{
			Name:      "test-cluster",
			Namespace: ns,
		})
		c.Spec.Services.Inference.PretrainedModels =
			append(c.Spec.Services.Inference.PretrainedModels,
				v1beta1.PretrainedModelReference{
					Name: model.Name,
				},
			)
		createCluster(c)

		By("waiting for the model deployment to be created")
		req, err := labels.NewRequirement(
			resources.PretrainedModelLabel, selection.In, []string{
				model.Name,
			})
		Expect(err).NotTo(HaveOccurred())
		Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
			LabelSelector: labels.NewSelector().Add(*req),
			Namespace:     cluster.Namespace,
		})).Should(HaveLen(1))

		By("deleting the model from the opnicluster")
		updateObject(cluster, func(c *v1beta1.OpniCluster) {
			c.Spec.Services.Inference.PretrainedModels =
				c.Spec.Services.Inference.PretrainedModels[:0]
		})

		By("verifying the model deployment is deleted")
		Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
			LabelSelector: labels.NewSelector().Add(*req),
			Namespace:     cluster.Namespace,
		})).Should(BeEmpty())

		By("adding the model back to the opnicluster")
		updateObject(cluster, func(c *v1beta1.OpniCluster) {
			c.Spec.Services.Inference.PretrainedModels =
				append(c.Spec.Services.Inference.PretrainedModels,
					v1beta1.PretrainedModelReference{
						Name: model.Name,
					})
		})

		By("verifying the model deployment is created")
		Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
			LabelSelector: labels.NewSelector().Add(*req),
			Namespace:     cluster.Namespace,
		})).Should(HaveLen(1))

		By("deleting the opnicluster")
		Expect(k8sClient.Delete(context.Background(), cluster)).To(Succeed())

		// Look up the matching pretrainedmodel
		By("verifying the pretrainedmodel was not deleted")
		Consistently(Object(&v1beta1.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-model",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			Not(HaveOwner(cluster)),
		))

		By("verifying the model deployment is deleted")
		Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
			LabelSelector: labels.NewSelector().Add(*req),
			Namespace:     cluster.Namespace,
		})).Should(BeEmpty())
	})
	It("should handle invalid pretrained models", func() {
		By("creating an opnicluster with an invalid pretrained model")
		createCluster(buildCluster(opniClusterOpts{
			Name:   "test-cluster",
			Models: []string{"invalid-model"},
		}))

		By("verifying the cluster is not in a healthy state")
		req, err := labels.NewRequirement(
			resources.PretrainedModelLabel, selection.In, []string{
				"invalid-model",
			})
		Expect(err).NotTo(HaveOccurred())
		done := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(done)
			Consistently(List(&appsv1.DeploymentList{}, &client.ListOptions{
				LabelSelector: labels.NewSelector().Add(*req),
				Namespace:     cluster.Namespace,
			})).Should(BeEmpty())
		}()
		// Ensure the state is set to error
		Eventually(Object(cluster)).Should(MatchStatus(
			func(s v1beta1.OpniClusterStatus) bool {
				return s.State == v1beta1.OpniClusterStateError &&
					len(s.Conditions) > 0
			},
		))
		<-done

		By("creating the previously invalid model")
		// Create the previouslinvalid model
		model := v1beta1.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-model",
				Namespace: cluster.Namespace,
			},
			Spec: v1beta1.PretrainedModelSpec{
				ModelSource: v1beta1.ModelSource{
					HTTP: &v1beta1.HTTPSource{
						URL: "http://invalid-model",
					},
				},
			},
		}

		By("verifying the cluster state has changed")
		Expect(k8sClient.Create(context.Background(), &model)).To(Succeed())
		Eventually(Object(cluster)).Should(MatchStatus(
			func(s v1beta1.OpniClusterStatus) bool {
				return s.State != v1beta1.OpniClusterStateError
			},
		))
	})
	// TODO: decide how to handle deleting pretrainedmodels in use
	PWhen("deleting a pretrained model while an opnicluster is using it", func() {
		PIt("should succeed", func() {})
		PIt("should delete the inference service", func() {})
		PIt("should delete the pretrainedmodel resource", func() {})
		PIt("should cause the opnicluster to report a status condition", func() {})
	})

	It("should create and reconcile an elastic cluster", func() {
		By("creating an opnicluster")
		c := buildCluster(opniClusterOpts{
			Name:                "test-elastic",
			DisableOpniServices: true,
		})
		c.Spec.Elastic = v1beta1.ElasticSpec{
			Version: "1.0.0",
			Persistence: &v1beta1.PersistenceSpec{
				Enabled:          true,
				StorageClassName: pointer.String("test-storageclass"),
			},
			Workloads: v1beta1.ElasticWorkloadSpec{
				Master: v1beta1.ElasticWorkloadMasterSpec{
					Replicas: pointer.Int32(1),
				},
				Data: v1beta1.ElasticWorkloadDataSpec{
					DedicatedPod: true,
					Replicas:     pointer.Int32(3),
				},
				Client: v1beta1.ElasticWorkloadClientSpec{
					DedicatedPod: true,
					Replicas:     pointer.Int32(5),
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("10Gi"),
						},
					},
				},
				Kibana: v1beta1.ElasticWorkloadKibanaSpec{
					Replicas: pointer.Int32(7),
				},
			},
		}
		createCluster(c)

		By("checking if the elastic deployments are created")
		wg := sync.WaitGroup{}
		wg.Add(4)
		go func() {
			defer GinkgoRecover()
			defer wg.Done()
			Eventually(Object(&appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-es-master",
					Namespace: cluster.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(cluster),
				HaveLabels(
					"app", "opendistro-es",
					"role", "master",
				),
				HaveReplicaCount(1),
				HaveMatchingVolume(And(
					HaveName("config"),
					HaveVolumeSource("Secret"),
				)),
				HaveMatchingContainer(And(
					HaveName("elasticsearch"),
					HaveImage("docker.io/amazon/opendistro-for-elasticsearch:1.0.0"),
					HaveEnv("node.master", "true"),
					HavePorts("transport", "http", "metrics", "rca"),
					HaveVolumeMounts("config", "data"),
				)),
				HaveMatchingPersistentVolume(And(
					HaveName("data"),
					HaveStorageClass("test-storageclass"),
				)),
			))
		}()
		go func() {
			defer GinkgoRecover()
			defer wg.Done()
			Eventually(Object(&appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-es-data",
					Namespace: cluster.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(cluster),
				HaveLabels(
					"app", "opendistro-es",
					"role", "data",
				),
				HaveReplicaCount(3),
				HaveMatchingContainer(And(
					HaveName("elasticsearch"),
					HaveImage("docker.io/amazon/opendistro-for-elasticsearch:1.0.0"),
					HaveEnv("node.data", "true"),
					HavePorts("transport"),
					HaveVolumeMounts("config", "data"),
				)),
				HaveMatchingPersistentVolume(And(
					HaveName("data"),
					HaveStorageClass("test-storageclass"),
				)),
			))
		}()
		go func() {
			defer GinkgoRecover()
			defer wg.Done()
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-es-client",
					Namespace: cluster.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(cluster),
				HaveLabels(
					"app", "opendistro-es",
					"role", "client",
				),
				HaveReplicaCount(5),
				HaveMatchingContainer(And(
					HaveName("elasticsearch"),
					HaveImage("docker.io/amazon/opendistro-for-elasticsearch:1.0.0"),
					HaveEnv(
						"node.ingest", "true",
						"ES_JAVA_OPTS", "-Xms5369m -Xmx5369m",
					),
					HavePorts("transport", "http", "metrics", "rca"),
					HaveVolumeMounts("config"),
					Not(HaveVolumeMounts("data")),
				)),
			))
		}()
		go func() {
			defer GinkgoRecover()
			defer wg.Done()
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-es-kibana",
					Namespace: cluster.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(cluster),
				HaveLabels(
					"app", "opendistro-es",
					"role", "kibana",
				),
				HaveReplicaCount(7),
				HaveMatchingContainer(And(
					HaveName("opni-es-kibana"),
					HaveImage("docker.io/amazon/opendistro-for-elasticsearch-kibana:1.0.0"),
					HaveEnv(
						"CLUSTER_NAME", nil,
						"ELASTICSEARCH_HOSTS", nil,
					),
					HavePorts("http"),
					Not(HaveVolumeMounts("config", "data")),
				)),
			))
		}()
		wg.Wait()
	})
})
