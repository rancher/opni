package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/intstr"
	opsterv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/resources"
)

var _ = Describe("AI OpniCluster Controller", Ordered, Label("controller"), func() {
	cluster := &aiv1beta1.OpniCluster{}

	createCluster := func(c *aiv1beta1.OpniCluster) {
		opensearch := &opsterv1.OpenSearchCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: c.Namespace,
			},
			Spec: opsterv1.ClusterSpec{
				General: opsterv1.GeneralConfig{
					Version: "1.0.0",
				},
				NodePools: []opsterv1.NodePool{
					{
						Component: "test",
						Roles: []string{
							"data",
							"master",
						},
					},
				},
			},
		}
		err := k8sClient.Create(context.Background(), opensearch)
		Expect(err).NotTo(HaveOccurred())
		Eventually(Object(opensearch)).Should(Exist())

		c.Spec.Opensearch = &opnimeta.OpensearchClusterRef{
			Name:      "test-cluster",
			Namespace: c.Namespace,
		}

		nats := &opnicorev1beta1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: c.Namespace,
			},
			Spec: opnicorev1beta1.NatsSpec{
				Replicas:   lo.ToPtr(int32(3)),
				AuthMethod: opnicorev1beta1.NatsAuthNkey,
			},
		}
		err = k8sClient.Create(context.Background(), nats)
		Expect(err).NotTo(HaveOccurred())
		Eventually(Object(nats)).Should(Exist())

		c.Spec.NatsRef = corev1.LocalObjectReference{
			Name: nats.Name,
		}

		err = k8sClient.Create(context.Background(), c)
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
		wg := &sync.WaitGroup{}
		createCluster(buildAICluster(opniClusterOpts{Name: "test"}))

		for _, kind := range []aiv1beta1.ServiceKind{
			aiv1beta1.DrainService,
			aiv1beta1.InferenceService,
			aiv1beta1.PayloadReceiverService,
			aiv1beta1.PreprocessingService,
			aiv1beta1.GPUControllerService,
			aiv1beta1.MetricsService,
			aiv1beta1.TrainingControllerService,
		} {
			wg.Add(1)

			By(fmt.Sprintf("checking %s service metadata and containers", kind.String()))
			go func(kind aiv1beta1.ServiceKind) {
				defer GinkgoRecover()
				defer wg.Done()
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
				Eventually(Object(&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      kind.ServiceName(),
						Namespace: cluster.Namespace,
					},
				})).Should(ExistAnd(
					HaveImagePullSecrets("lorem-ipsum"),
					HaveNodeSelector("foo", "bar"),
					HaveTolerations("foo"),
				))
			}(kind)
		}
		wg.Wait()

		By("checking the gpu service data mount exists")
		Eventually(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiv1beta1.GPUControllerService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveMatchingVolume(And(
				HaveName("data"),
				HaveVolumeSource("EmptyDir"),
			)),
			HaveMatchingContainer(And(
				HaveName("gpu-service-worker"),
				HaveVolumeMounts(corev1.VolumeMount{
					Name:      "data",
					MountPath: "/var/opni-data",
				}),
			)),
		))
		By("checking that pretrained model services are not created yet")
		// Identify pretrained model services with the label "opni.io/pretrained-model"
		req, err := labels.NewRequirement(
			resources.PretrainedModelLabel, selection.Exists, nil)
		Expect(err).NotTo(HaveOccurred())
		Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
			Namespace:     cluster.Namespace,
			LabelSelector: labels.NewSelector().Add(*req),
		})).Should(BeEmpty())

		By("checking hyperparameters config")
		defaultHyperParameters, _ := json.MarshalIndent(map[string]intstr.IntOrString{
			"modelThreshold": intstr.FromString("0.7"),
			"minLogTokens":   intstr.FromInt(1),
		}, "", "  ")
		Eventually(Object(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-nulog-hyperparameters",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveData("hyperparameters.json", string(defaultHyperParameters)),
			HaveOwner(cluster),
		))
		Eventually(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiv1beta1.InferenceService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveMatchingVolume(And(
				HaveName("hyperparameters"),
				HaveVolumeSource("ConfigMap"),
			)),
			HaveMatchingContainer(
				HaveVolumeMounts(corev1.VolumeMount{
					Name:      "hyperparameters",
					MountPath: "/etc/opni/hyperparameters.json",
					SubPath:   "hyperparameters.json",
					ReadOnly:  true,
				}),
			),
		))
		Eventually(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiv1beta1.GPUControllerService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveMatchingVolume(And(
				HaveName("hyperparameters"),
				HaveVolumeSource("ConfigMap"),
			)),
			HaveMatchingContainer(And(
				HaveName("gpu-service-worker"),
				HaveVolumeMounts(corev1.VolumeMount{
					Name:      "hyperparameters",
					MountPath: "/etc/opni/hyperparameters.json",
					SubPath:   "hyperparameters.json",
					ReadOnly:  true,
				}),
			)),
		))
	})
	It("should not create the metrics service when the prometheus endpoint is invalid", func() {
		By("waiting for the cluster to be created")
		createCluster(buildAICluster(opniClusterOpts{
			Name:               "test",
			PrometheusEndpoint: "badendpoint",
		}))
		By("ensuring the metrics service is not created.")
		Consistently(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiv1beta1.MetricsService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).ShouldNot(Exist())
	})
	It("should create monitoring objects when prometheusRef is defined", func() {
		By("creating a prometheus")
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "prometheus-new",
			},
		}
		Expect(k8sClient.Create(context.Background(), namespace)).To(Succeed())
		prometheus := &monitoringv1.Prometheus{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-prometheus",
				Namespace: "prometheus-new",
			},
			Spec: monitoringv1.PrometheusSpec{
				CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
					ExternalURL: "http://prometheus-test.prometheus",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    *resource.NewMilliQuantity(250, resource.DecimalSI),
							corev1.ResourceMemory: *resource.NewScaledQuantity(250, resource.Mega),
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), prometheus)).To(Succeed())

		By("creating a cluster")
		createCluster(buildAICluster(opniClusterOpts{
			Name:             "test-cluster",
			UsePrometheusRef: true,
		}))

		By("checking the metrics service is created")
		Eventually(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiv1beta1.MetricsService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveLabels(
				resources.AppNameLabel, aiv1beta1.MetricsService.ServiceName(),
				resources.ServiceLabel, aiv1beta1.MetricsService.String(),
				resources.PartOfLabel, "opni",
			),
			HaveOwner(cluster),
			HaveMatchingVolume(And(
				HaveName("test-volume"),
				HaveVolumeSource("EmptyDir"),
			)),
			HaveMatchingContainer(And(
				HaveVolumeMounts("test-volume"),
			)),
		))
		Eventually(Object(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiv1beta1.MetricsService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveLabels(
				resources.AppNameLabel, aiv1beta1.MetricsService.ServiceName(),
				resources.ServiceLabel, aiv1beta1.MetricsService.String(),
				resources.PartOfLabel, "opni",
			),
			HavePorts("metrics"),
			HaveOwner(cluster),
		))
		By("checking the monitoring resources are created")
		Eventually(Object(&monitoringv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiv1beta1.MetricsService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
		))
		Eventually(Object(&monitoringv1.PrometheusRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", aiv1beta1.MetricsService.ServiceName(), generateSHAID(cluster.Name, cluster.Namespace)),
				Namespace: "prometheus-new",
			},
		})).Should(ExistAnd(
			Not(HaveOwner(cluster)),
		))
	})
	It("should clean up the prometheusRule", func() {
		By("deleting the cluster")
		Expect(k8sClient.Delete(context.Background(), cluster)).To(Succeed())

		By("checking the prometheus rule is deleted")
		Eventually(Object(&monitoringv1.PrometheusRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", aiv1beta1.MetricsService.ServiceName(), generateSHAID(cluster.Name, cluster.Namespace)),
				Namespace: "prometheus",
			},
		})).ShouldNot(Exist())
	})
	It("should create inference services for pretrained models", func() {
		ns := makeTestNamespace()
		// Not testing that the pretrained model controller works here, as that
		// is tested in the pretrained model controller test.
		By("creating a pretrained model")
		Expect(k8sClient.Create(context.Background(), &aiv1beta1.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-model",
				Namespace: ns,
			},
			Spec: aiv1beta1.PretrainedModelSpec{
				ModelSource: aiv1beta1.ModelSource{
					HTTP: &aiv1beta1.HTTPSource{
						URL: "https://foo.bar/model.tar.gz",
					},
				},
			},
		})).To(Succeed())

		By("creating a cluster")
		createCluster(buildAICluster(opniClusterOpts{
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

	Specify("providing hyperparameters should work", func() {
		By("creating a cluster")
		createCluster(buildAICluster(opniClusterOpts{Name: "test"}))

		By("adding hyperparameters")
		testData := map[string]intstr.IntOrString{
			"meaning-of-life": intstr.FromInt(42),
		}
		updateObject(cluster, func(c *aiv1beta1.OpniCluster) {
			c.Spec.NulogHyperparameters = testData
		})
		testBytes, _ := json.MarshalIndent(testData, "", "  ")
		Eventually(Object(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-nulog-hyperparameters",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveData("hyperparameters.json", string(testBytes)),
			HaveOwner(cluster),
		))
	})
	Context("pretrained models should function in various configurations", func() {
		It("should ignore duplicate model names", func() {
			createCluster(buildAICluster(opniClusterOpts{
				Name: "test",
				Models: []string{
					"test-model",
					"test-model",
				},
			}))
			Expect(k8sClient.Create(context.Background(), &aiv1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model",
					Namespace: cluster.Namespace,
				},
				Spec: aiv1beta1.PretrainedModelSpec{
					ModelSource: aiv1beta1.ModelSource{
						HTTP: &aiv1beta1.HTTPSource{
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
			model1 := aiv1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-1",
					Namespace: ns,
				},
				Spec: aiv1beta1.PretrainedModelSpec{
					ModelSource: aiv1beta1.ModelSource{
						HTTP: &aiv1beta1.HTTPSource{
							URL: "https://foo.bar/model.tar.gz",
						},
					},
					Hyperparameters: map[string]intstr.IntOrString{
						"foo": intstr.FromString("0.1"),
					},
				},
			}
			model2 := aiv1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-2",
					Namespace: ns,
				},
				Spec: aiv1beta1.PretrainedModelSpec{
					ModelSource: aiv1beta1.ModelSource{
						HTTP: &aiv1beta1.HTTPSource{
							URL: "https://bar.baz/model.tar.gz",
						},
					},
					Hyperparameters: map[string]intstr.IntOrString{
						"bar": intstr.FromString("0.2"),
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &model1)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), &model2)).To(Succeed())
			// Create cluster with both models
			createCluster(buildAICluster(opniClusterOpts{
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
			model1 := aiv1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-1",
					Namespace: ns,
				},
				Spec: aiv1beta1.PretrainedModelSpec{
					ModelSource: aiv1beta1.ModelSource{
						HTTP: &aiv1beta1.HTTPSource{
							URL: "https://foo.bar/model.tar.gz",
						},
					},
					Hyperparameters: map[string]intstr.IntOrString{
						"foo": intstr.FromString("0.1"),
					},
				},
			}
			model2 := aiv1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-2",
					Namespace: ns,
				},
				Spec: aiv1beta1.PretrainedModelSpec{
					ModelSource: aiv1beta1.ModelSource{
						Container: &aiv1beta1.ContainerSource{
							Image: "gcr.io/foo/bar:latest",
						},
					},
					Hyperparameters: map[string]intstr.IntOrString{
						"baz": intstr.FromString("0.3"),
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &model1)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), &model2)).To(Succeed())
			// Create cluster with both models
			createCluster(buildAICluster(opniClusterOpts{
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
		createCluster(buildAICluster(opniClusterOpts{
			Name: "test-cluster",
		}))

		By("creating a model")
		model := aiv1beta1.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-model",
				Namespace: cluster.Namespace,
			},
			Spec: aiv1beta1.PretrainedModelSpec{
				ModelSource: aiv1beta1.ModelSource{
					HTTP: &aiv1beta1.HTTPSource{
						URL: "https://foo.bar/model.tar.gz",
					},
				},
				Hyperparameters: map[string]intstr.IntOrString{
					"foo": intstr.FromString("0.1"),
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), &model)).To(Succeed())

		By("adding the model to the opnicluster")
		updateObject(cluster, func(c *aiv1beta1.OpniCluster) {
			c.Spec.Services.Inference.PretrainedModels =
				append(c.Spec.Services.Inference.PretrainedModels,
					corev1.LocalObjectReference{
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
		model := aiv1beta1.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-model",
				Namespace: ns,
			},
			Spec: aiv1beta1.PretrainedModelSpec{
				ModelSource: aiv1beta1.ModelSource{
					HTTP: &aiv1beta1.HTTPSource{
						URL: "https://foo.bar/model.tar.gz",
					},
				},
				Hyperparameters: map[string]intstr.IntOrString{
					"foo": intstr.FromString("0.1"),
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), &model)).To(Succeed())

		By("creating an opnicluster with the model")
		c := buildAICluster(opniClusterOpts{
			Name:      "test-cluster",
			Namespace: ns,
		})
		c.Spec.Services.Inference.PretrainedModels =
			append(c.Spec.Services.Inference.PretrainedModels,
				corev1.LocalObjectReference{
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
		updateObject(cluster, func(c *aiv1beta1.OpniCluster) {
			c.Spec.Services.Inference.PretrainedModels =
				c.Spec.Services.Inference.PretrainedModels[:0]
		})

		By("verifying the model deployment is deleted")
		Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
			LabelSelector: labels.NewSelector().Add(*req),
			Namespace:     cluster.Namespace,
		})).Should(BeEmpty())

		By("adding the model back to the opnicluster")
		updateObject(cluster, func(c *aiv1beta1.OpniCluster) {
			c.Spec.Services.Inference.PretrainedModels =
				append(c.Spec.Services.Inference.PretrainedModels,
					corev1.LocalObjectReference{
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
		Consistently(Object(&aiv1beta1.PretrainedModel{
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
		createCluster(buildAICluster(opniClusterOpts{
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
			func(s aiv1beta1.OpniClusterStatus) bool {
				return s.State == aiv1beta1.OpniClusterStateError &&
					len(s.Conditions) > 0
			},
		))
		<-done

		By("creating the previously invalid model")
		// Create the previouslinvalid model
		model := aiv1beta1.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-model",
				Namespace: cluster.Namespace,
			},
			Spec: aiv1beta1.PretrainedModelSpec{
				ModelSource: aiv1beta1.ModelSource{
					HTTP: &aiv1beta1.HTTPSource{
						URL: "http://invalid-model",
					},
				},
			},
		}

		By("verifying the cluster state has changed")
		Expect(k8sClient.Create(context.Background(), &model)).To(Succeed())
		Eventually(Object(cluster)).Should(MatchStatus(
			func(s aiv1beta1.OpniClusterStatus) bool {
				return s.State != aiv1beta1.OpniClusterStateError
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

	It("should reconcile internal s3 resources", func() {
		By("creating the opnicluster")
		c := buildAICluster(opniClusterOpts{
			Name: "test",
		})
		c.Spec.S3.Internal = &aiv1beta1.InternalSpec{
			Persistence: &opnimeta.PersistenceSpec{
				Enabled:          true,
				StorageClassName: lo.ToPtr("testing"),
				Request:          resource.MustParse("64Gi"),
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany,
				},
			},
		}
		createCluster(c)

		By("checking if the seaweed resources are created")
		Eventually(Object(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-seaweed",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveLabels("app", "seaweed"),
			HaveMatchingContainer(And(
				HaveVolumeMounts("opni-seaweed-data"),
			)),
			HaveMatchingPersistentVolume(And(
				HaveName("opni-seaweed-data"),
				HaveStorageClass("testing"),
			)),
		))
		Eventually(Object(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-seaweed-s3",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveLabels("app", "seaweed"),
			HavePorts("s3"),
		))
		Eventually(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-seaweed-config",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveLabels("app", "seaweed"),
			HaveData(
				"accessKey", nil,
				"secretKey", nil,
				"config.json", nil,
			),
		))

		By("checking that the cluster status contains key references")
		Eventually(Object(cluster)).Should(MatchStatus(func(status aiv1beta1.OpniClusterStatus) bool {
			return status.Auth.S3AccessKey != nil &&
				status.Auth.S3SecretKey != nil &&
				status.Auth.S3Endpoint != ""
		}))
	})

	It("should reconcile external s3 resources", func() {
		By("creating the opnicluster with a missing secret")
		c := buildAICluster(opniClusterOpts{
			Name: "test",
		})
		c.Spec.S3.External = &aiv1beta1.ExternalSpec{
			Endpoint: "http://s3.amazonaws.biz",
			Credentials: &corev1.SecretReference{
				Name: "missing-secret",
			},
		}
		createCluster(c)

		By("checking that the external key secret is not created")
		Consistently(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-secret",
				Namespace: cluster.Namespace,
			},
		})).ShouldNot(Exist())

		By("checking that s3 info in the cluster status should be unset")
		Eventually(Object(cluster)).Should(MatchStatus(func(status aiv1beta1.OpniClusterStatus) bool {
			return status.Auth.S3Endpoint == "" &&
				status.Auth.S3AccessKey == nil &&
				status.Auth.S3SecretKey == nil
		}))

		By("creating the secret")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-secret",
				Namespace: c.Namespace,
			},
			Data: map[string][]byte{
				"accessKey": []byte("access-key"),
				"secretKey": []byte("secret-key"),
			},
		}
		Expect(k8sClient.Create(context.Background(), secret)).To(Succeed())

		By("checking that the secret is created")
		Eventually(Object(secret)).Should(Exist())

		By("checking that seaweed is not deployed")
		Consistently(Object(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-seaweed",
				Namespace: cluster.Namespace,
			},
		})).ShouldNot(Exist())
		Consistently(Object(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-seaweed-s3",
				Namespace: cluster.Namespace,
			},
		})).ShouldNot(Exist())

		By("checking that s3 info in the cluster status should be set")
		Eventually(Object(cluster), 60*time.Second).Should(MatchStatus(func(status aiv1beta1.OpniClusterStatus) bool {
			return status.Auth.S3Endpoint == "http://s3.amazonaws.biz" &&
				status.Auth.S3AccessKey != nil &&
				status.Auth.S3SecretKey != nil
		}))

	})

	It("should reconcile internal s3 resources", func() {
		By("creating the cluster with internal s3")
		c := buildAICluster(opniClusterOpts{
			Name: "test",
		})
		c.Spec.S3.Internal = &aiv1beta1.InternalSpec{}
		createCluster(c)

		By("checking the seaweedfs statefulset is created")
		Eventually(Object(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-seaweed",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
		))

		By("creating the s3 service")
		Eventually(Object(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-seaweed-s3",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HavePorts("s3"),
		))

		By("checking that s3 info in the cluster status should be set")
		Eventually(Object(cluster)).Should(MatchStatus(func(status aiv1beta1.OpniClusterStatus) bool {
			return status.Auth.S3Endpoint == fmt.Sprintf("http://opni-seaweed-s3.%s.svc", cluster.Namespace) &&
				status.Auth.S3AccessKey != nil &&
				status.Auth.S3SecretKey != nil
		}))
	})
})
