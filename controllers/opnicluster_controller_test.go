package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
)

var _ = Describe("OpniCluster Controller", Ordered, Label("controller", "deprecated"), func() {
	cluster := &v1beta2.OpniCluster{}

	createCluster := func(c *v1beta2.OpniCluster) {
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
		wg := &sync.WaitGroup{}
		createCluster(buildCluster(opniClusterOpts{Name: "test"}))

		for _, kind := range []v1beta2.ServiceKind{
			v1beta2.PretrainedDrainService,
			//v1beta2.InferenceService,
			v1beta2.PayloadReceiverService,
			v1beta2.PreprocessingService,
			//v1beta2.GPUControllerService,
			v1beta2.MetricsService,
		} {
			wg.Add(1)

			By(fmt.Sprintf("checking %s service metadata and containers", kind.String()))
			go func(kind v1beta2.ServiceKind) {
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
					HaveMatchingContainer(
						HaveImage(fmt.Sprintf("docker.biz/rancher/%s:test", kind.ImageName()), corev1.PullNever),
					),
					HaveImagePullSecrets("lorem-ipsum"),
					HaveNodeSelector("foo", "bar"),
					HaveTolerations("foo"),
				))
			}(kind)
		}
		wg.Wait()

		// By("checking the gpu service data mount exists")
		// Eventually(Object(&appsv1.Deployment{
		// 	ObjectMeta: metav1.ObjectMeta{
		// 		Name:      v1beta2.GPUControllerService.ServiceName(),
		// 		Namespace: cluster.Namespace,
		// 	},
		// })).Should(ExistAnd(
		// 	HaveMatchingVolume(And(
		// 		HaveName("data"),
		// 		HaveVolumeSource("EmptyDir"),
		// 	)),
		// 	HaveMatchingContainer(And(
		// 		HaveName(v1beta2.GPUControllerService.ServiceName()),
		// 		HaveVolumeMounts(corev1.VolumeMount{
		// 			Name:      "data",
		// 			MountPath: "/var/opni-data",
		// 		}),
		// 	)),
		// 	HaveMatchingContainer(And(
		// 		HaveName("gpu-service-worker"),
		// 		HaveVolumeMounts(corev1.VolumeMount{
		// 			Name:      "data",
		// 			MountPath: "/var/opni-data",
		// 		}),
		// 	)),
		// ))
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
		})).Should(ExistAnd(
			HaveReplicaCount(3),
			HaveNodeSelector("foo", "bar"),
			HaveTolerations("foo"),
			HaveOwner(cluster),
		))

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
		By("checking hyperparameters config")
		defaultHyperParameters, _ := json.MarshalIndent(map[string]intstr.IntOrString{
			"modelThreshold": intstr.FromString("0.5"),
			"minLogTokens":   intstr.FromInt(5),
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
		// Eventually(Object(&appsv1.Deployment{
		// 	ObjectMeta: metav1.ObjectMeta{
		// 		Name:      v1beta2.InferenceService.ServiceName(),
		// 		Namespace: cluster.Namespace,
		// 	},
		// })).Should(ExistAnd(
		// 	HaveMatchingVolume(And(
		// 		HaveName("hyperparameters"),
		// 		HaveVolumeSource("ConfigMap"),
		// 	)),
		// 	HaveMatchingContainer(
		// 		HaveVolumeMounts(corev1.VolumeMount{
		// 			Name:      "hyperparameters",
		// 			MountPath: "/etc/opni/hyperparameters.json",
		// 			SubPath:   "hyperparameters.json",
		// 			ReadOnly:  true,
		// 		}),
		// 	),
		// ))
		// Eventually(Object(&appsv1.Deployment{
		// 	ObjectMeta: metav1.ObjectMeta{
		// 		Name:      v1beta2.GPUControllerService.ServiceName(),
		// 		Namespace: cluster.Namespace,
		// 	},
		// })).Should(ExistAnd(
		// 	HaveMatchingVolume(And(
		// 		HaveName("hyperparameters"),
		// 		HaveVolumeSource("ConfigMap"),
		// 	)),
		// 	HaveMatchingContainer(And(
		// 		HaveName("gpu-service-worker"),
		// 		HaveVolumeMounts(corev1.VolumeMount{
		// 			Name:      "hyperparameters",
		// 			MountPath: "/etc/opni/hyperparameters.json",
		// 			SubPath:   "hyperparameters.json",
		// 			ReadOnly:  true,
		// 		}),
		// 	)),
		// ))
	})
	It("should not create the metrics service when the prometheus endpoint is invalid", func() {
		By("waiting for the cluster to be created")
		createCluster(buildCluster(opniClusterOpts{
			Name:               "test",
			PrometheusEndpoint: "badendpoint",
		}))
		By("ensuring the metrics service is not created.")
		Consistently(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      v1beta2.MetricsService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).ShouldNot(Exist())
	})
	It("should create monitoring objects when prometheusRef is defined", func() {
		By("creating a prometheus")
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "prometheus",
			},
		}
		Expect(k8sClient.Create(context.Background(), namespace)).To(Succeed())
		prometheus := &monitoringv1.Prometheus{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-prometheus",
				Namespace: "prometheus",
			},
			Spec: monitoringv1.PrometheusSpec{
				CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
					ExternalURL:    "http://prometheus-test.prometheus",
					EnableAdminAPI: false,
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
		createCluster(buildCluster(opniClusterOpts{
			Name:             "test-cluster",
			UsePrometheusRef: true,
		}))

		By("checking the metrics service is created")
		Eventually(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      v1beta2.MetricsService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveLabels(
				resources.AppNameLabel, v1beta2.MetricsService.ServiceName(),
				resources.ServiceLabel, v1beta2.MetricsService.String(),
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
				Name:      v1beta2.MetricsService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveLabels(
				resources.AppNameLabel, v1beta2.MetricsService.ServiceName(),
				resources.ServiceLabel, v1beta2.MetricsService.String(),
				resources.PartOfLabel, "opni",
			),
			HavePorts("metrics"),
			HaveOwner(cluster),
		))
		By("checking the monitoring resources are created")
		Eventually(Object(&monitoringv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      v1beta2.MetricsService.ServiceName(),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
		))
		Eventually(Object(&monitoringv1.PrometheusRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", v1beta2.MetricsService.ServiceName(), generateSHAID(cluster.Name, cluster.Namespace)),
				Namespace: "prometheus",
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
				Name:      fmt.Sprintf("%s-%s", v1beta2.MetricsService.ServiceName(), generateSHAID(cluster.Name, cluster.Namespace)),
				Namespace: "prometheus",
			},
		})).ShouldNot(Exist())
	})
	It("should create inference services for pretrained models", func() {
		ns := makeTestNamespace()
		// Not testing that the pretrained model controller works here, as that
		// is tested in the pretrained model controller test.
		By("creating a pretrained model")
		Expect(k8sClient.Create(context.Background(), &v1beta2.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-model",
				Namespace: ns,
			},
			Spec: v1beta2.PretrainedModelSpec{
				ModelSource: v1beta2.ModelSource{
					HTTP: &v1beta2.HTTPSource{
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
		c.Spec.Nats.AuthMethod = v1beta2.NatsAuthNkey

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
			MatchStatus(func(s v1beta2.OpniClusterStatus) bool {
				return s.Auth.NKeyUser != "" &&
					s.Auth.NatsAuthSecretKeyRef != nil &&
					s.Auth.NatsAuthSecretKeyRef.Name == fmt.Sprintf("%s-nats-client", cluster.Name)
			}),
		)
	})

	Specify("providing hyperparameters should work", func() {
		testData := map[string]intstr.IntOrString{
			"meaning-of-life": intstr.FromInt(42),
		}
		updateObject(cluster, func(c *v1beta2.OpniCluster) {
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
			createCluster(buildCluster(opniClusterOpts{
				Name: "test",
				Models: []string{
					"test-model",
					"test-model",
				},
			}))
			Expect(k8sClient.Create(context.Background(), &v1beta2.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model",
					Namespace: cluster.Namespace,
				},
				Spec: v1beta2.PretrainedModelSpec{
					ModelSource: v1beta2.ModelSource{
						HTTP: &v1beta2.HTTPSource{
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
			model1 := v1beta2.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-1",
					Namespace: ns,
				},
				Spec: v1beta2.PretrainedModelSpec{
					ModelSource: v1beta2.ModelSource{
						HTTP: &v1beta2.HTTPSource{
							URL: "https://foo.bar/model.tar.gz",
						},
					},
					Hyperparameters: map[string]intstr.IntOrString{
						"foo": intstr.FromString("0.1"),
					},
				},
			}
			model2 := v1beta2.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-2",
					Namespace: ns,
				},
				Spec: v1beta2.PretrainedModelSpec{
					ModelSource: v1beta2.ModelSource{
						HTTP: &v1beta2.HTTPSource{
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
			model1 := v1beta2.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-1",
					Namespace: ns,
				},
				Spec: v1beta2.PretrainedModelSpec{
					ModelSource: v1beta2.ModelSource{
						HTTP: &v1beta2.HTTPSource{
							URL: "https://foo.bar/model.tar.gz",
						},
					},
					Hyperparameters: map[string]intstr.IntOrString{
						"foo": intstr.FromString("0.1"),
					},
				},
			}
			model2 := v1beta2.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-2",
					Namespace: ns,
				},
				Spec: v1beta2.PretrainedModelSpec{
					ModelSource: v1beta2.ModelSource{
						Container: &v1beta2.ContainerSource{
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
		model := v1beta2.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-model",
				Namespace: cluster.Namespace,
			},
			Spec: v1beta2.PretrainedModelSpec{
				ModelSource: v1beta2.ModelSource{
					HTTP: &v1beta2.HTTPSource{
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
		updateObject(cluster, func(c *v1beta2.OpniCluster) {
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
		model := v1beta2.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-model",
				Namespace: ns,
			},
			Spec: v1beta2.PretrainedModelSpec{
				ModelSource: v1beta2.ModelSource{
					HTTP: &v1beta2.HTTPSource{
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
		c := buildCluster(opniClusterOpts{
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
		updateObject(cluster, func(c *v1beta2.OpniCluster) {
			c.Spec.Services.Inference.PretrainedModels =
				c.Spec.Services.Inference.PretrainedModels[:0]
		})

		By("verifying the model deployment is deleted")
		Eventually(List(&appsv1.DeploymentList{}, &client.ListOptions{
			LabelSelector: labels.NewSelector().Add(*req),
			Namespace:     cluster.Namespace,
		})).Should(BeEmpty())

		By("adding the model back to the opnicluster")
		updateObject(cluster, func(c *v1beta2.OpniCluster) {
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
		Consistently(Object(&v1beta2.PretrainedModel{
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
			func(s v1beta2.OpniClusterStatus) bool {
				return s.State == v1beta2.OpniClusterStateError &&
					len(s.Conditions) > 0
			},
		))
		<-done

		By("creating the previously invalid model")
		// Create the previouslinvalid model
		model := v1beta2.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-model",
				Namespace: cluster.Namespace,
			},
			Spec: v1beta2.PretrainedModelSpec{
				ModelSource: v1beta2.ModelSource{
					HTTP: &v1beta2.HTTPSource{
						URL: "http://invalid-model",
					},
				},
			},
		}

		By("verifying the cluster state has changed")
		Expect(k8sClient.Create(context.Background(), &model)).To(Succeed())
		Eventually(Object(cluster)).Should(MatchStatus(
			func(s v1beta2.OpniClusterStatus) bool {
				return s.State != v1beta2.OpniClusterStateError
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
		c.Spec.Opensearch = v1beta2.OpensearchClusterSpec{
			Version: "1.0.0",
			Persistence: &opnimeta.PersistenceSpec{
				Enabled:          true,
				StorageClassName: lo.ToPtr("test-storageclass"),
			},
			Workloads: v1beta2.OpensearchWorkloadSpec{
				Master: v1beta2.OpensearchWorkloadOptions{
					Replicas: lo.ToPtr[int32](1),
				},
				Data: v1beta2.OpensearchWorkloadOptions{
					Replicas: lo.ToPtr[int32](3),
				},
				Client: v1beta2.OpensearchWorkloadOptions{
					Replicas: lo.ToPtr[int32](5),
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("10Gi"),
						},
					},
					NodeSelector: map[string]string{
						"baz": "bat",
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "bar",
							Operator: corev1.TolerationOpExists,
						},
					},
				},
				Dashboards: v1beta2.OpensearchWorkloadOptions{
					Replicas: lo.ToPtr[int32](7),
				},
			},
		}
		createCluster(c)

		services := []string{"opni-es-data", "opni-es-client", "opni-es-discovery", "opni-es-kibana"}

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
					"app", "opensearch",
					"role", "master",
				),
				HaveReplicaCount(1),
				HaveNodeSelector("foo", "bar"),
				HaveTolerations("foo"),
				HaveMatchingVolume(And(
					HaveName("config"),
					HaveVolumeSource("Secret"),
				)),
				HaveMatchingVolume(And(
					HaveName("internalusers"),
					HaveVolumeSource("Secret"),
				)),
				HaveMatchingContainer(And(
					HaveName("elasticsearch"),
					HaveImage("docker.io/opensearchproject/opensearch:1.0.0"),
					HaveEnv("node.master", "true"),
					HavePorts("transport", "http", "metrics", "rca"),
					HaveVolumeMounts("config", "opni-es-data"),
					HaveVolumeMounts("internalusers", "opni-es-internalusers"),
				)),
				HaveMatchingPersistentVolume(And(
					HaveName("opni-es-data"),
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
					"app", "opensearch",
					"role", "data",
				),
				HaveReplicaCount(3),
				HaveNodeSelector("foo", "bar"),
				HaveTolerations("foo"),
				HaveMatchingContainer(And(
					HaveName("elasticsearch"),
					HaveImage("docker.io/opensearchproject/opensearch:1.0.0"),
					HaveEnv(
						"node.data", "true",
						"node.ingest", "true",
					),
					HavePorts("transport"),
					HaveVolumeMounts("config", "opni-es-data"),
					HaveVolumeMounts("internalusers", "opni-es-internalusers"),
				)),
				HaveMatchingPersistentVolume(And(
					HaveName("opni-es-data"),
					HaveStorageClass("test-storageclass"),
				)),
				HaveMatchingVolume(And(
					HaveName("internalusers"),
					HaveVolumeSource("Secret"),
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
					"app", "opensearch",
					"role", "client",
				),
				HaveReplicaCount(5),
				HaveNodeSelector("baz", "bat"),
				HaveTolerations("foo", "bar"),
				HaveMatchingContainer(And(
					HaveName("elasticsearch"),
					HaveImage("docker.io/opensearchproject/opensearch:1.0.0"),
					HaveEnv(
						"OPENSEARCH_JAVA_OPTS", "-Dlog4j2.formatMsgNoLookups=true -Xms5369m -Xmx5369m",
					),
					HavePorts("transport", "http", "metrics", "rca"),
					HaveVolumeMounts("config"),
					HaveVolumeMounts("internalusers", "opni-es-internalusers"),
					Not(HaveVolumeMounts("opni-es-data")),
				)),
				HaveMatchingVolume(And(
					HaveName("internalusers"),
					HaveVolumeSource("Secret"),
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
					"app", "opensearch",
					"role", "kibana",
				),
				HaveReplicaCount(7),
				HaveNodeSelector("foo", "bar"),
				HaveTolerations("foo"),
				HaveMatchingContainer(And(
					HaveName("opni-es-kibana"),
					HaveImage("docker.io/opensearchproject/opensearch-dashboards:1.0.0"),
					HaveEnv(
						"OPENSEARCH_HOSTS", nil,
					),
					HavePorts("http"),
					Not(HaveVolumeMounts("config", "opni-es-data")),
				)),
			))
		}()
		wg.Wait()

		By("checking that services are created")
		for _, svc := range services {
			Eventually(Object(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svc,
					Namespace: cluster.Namespace,
				},
			})).Should(ExistAnd(
				HaveOwner(cluster),
			))
		}

		By("checking that secrets are created")
		Eventually(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-es-config",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveData("logging.yml", nil),
		))
		Eventually(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-es-password",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveData("password", nil),
		))
		Eventually(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-es-internalusers",
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveData("internal_users.yml", nil),
		))

		By("adjusting the elastic workload replicas")
		updateObject(cluster, func(obj *v1beta2.OpniCluster) {
			obj.Spec.Opensearch.Workloads.Data.Replicas = lo.ToPtr[int32](1)
			obj.Spec.Opensearch.Workloads.Client.Replicas = lo.ToPtr[int32](1)
			obj.Spec.Opensearch.Workloads.Dashboards.Replicas = lo.ToPtr[int32](1)
		})

		By("checking that replica counts are updated")
		Eventually(Object(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-es-data",
				Namespace: cluster.Namespace,
			},
		})).Should(HaveReplicaCount(1))
		Eventually(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-es-client",
				Namespace: cluster.Namespace,
			},
		})).Should(HaveReplicaCount(1))
		Eventually(Object(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-es-kibana",
				Namespace: cluster.Namespace,
			},
		})).Should(HaveReplicaCount(1))

		By("deleting the opnicluster")
		Expect(k8sClient.Delete(context.Background(), cluster)).To(Succeed())

		statefulSets := []string{"opni-es-data", "opni-es-master", "opni-nats"}
		By("ensuring all elastic resources are deleted")
		for _, name := range statefulSets {
			Eventually(Object(&appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: cluster.Namespace,
				},
			})).ShouldNot(Exist())
		}
		deployments := []string{"opni-es-client", "opni-es-kibana"}
		for _, name := range deployments {
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: cluster.Namespace,
				},
			})).ShouldNot(Exist())
		}
		for _, name := range services {
			Eventually(Object(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: cluster.Namespace,
				},
			})).ShouldNot(Exist())
		}
		secrets := []string{"opni-es-config"}
		for _, name := range secrets {
			Eventually(Object(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: cluster.Namespace,
				},
			})).ShouldNot(Exist())
		}
	})

	It("should reconcile internal s3 resources", func() {
		By("creating the opnicluster")
		c := buildCluster(opniClusterOpts{
			Name: "test",
		})
		c.Spec.S3.Internal = &v1beta2.InternalSpec{
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
		Eventually(Object(cluster)).Should(MatchStatus(func(status v1beta2.OpniClusterStatus) bool {
			return status.Auth.S3AccessKey != nil &&
				status.Auth.S3SecretKey != nil &&
				status.Auth.S3Endpoint != ""
		}))
	})

	It("should reconcile external s3 resources", func() {
		By("creating the opnicluster with a missing secret")
		c := buildCluster(opniClusterOpts{
			Name: "test",
		})
		c.Spec.S3.External = &v1beta2.ExternalSpec{
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
		Eventually(Object(cluster)).Should(MatchStatus(func(status v1beta2.OpniClusterStatus) bool {
			return status.Auth.S3Endpoint == "" &&
				status.Auth.S3AccessKey == nil &&
				status.Auth.S3SecretKey == nil
		}))

		By("creating the secret")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-secret",
				Namespace: cluster.Namespace,
			},
			Data: map[string][]byte{
				"accessKey": []byte("access-key"),
				"secretKey": []byte("secret-key"),
			},
		}
		Expect(k8sClient.Create(context.Background(), secret)).To(Succeed())

		By("checking that the secret is created")
		Eventually(Object(secret)).Should(Exist())

		By("cbecking that seaweed is not deployed")
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
		Eventually(Object(cluster)).Should(MatchStatus(func(status v1beta2.OpniClusterStatus) bool {
			return status.Auth.S3Endpoint == "http://s3.amazonaws.biz" &&
				status.Auth.S3AccessKey != nil &&
				status.Auth.S3SecretKey != nil
		}))

	})

	It("should reconcile internal s3 resources", func() {
		By("creating the cluster with internal s3")
		c := buildCluster(opniClusterOpts{
			Name: "test",
		})
		c.Spec.S3.Internal = &v1beta2.InternalSpec{}
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
		Eventually(Object(cluster)).Should(MatchStatus(func(status v1beta2.OpniClusterStatus) bool {
			return status.Auth.S3Endpoint == fmt.Sprintf("http://opni-seaweed-s3.%s.svc", cluster.Namespace) &&
				status.Auth.S3AccessKey != nil &&
				status.Auth.S3SecretKey != nil
		}))
	})
})
