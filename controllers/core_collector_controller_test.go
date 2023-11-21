package controllers_test

import (
	"context"
	"fmt"

	opnimonitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
	"github.com/rancher/opni/pkg/otel"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promoperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Core Collector Controller", Ordered, Label("controller", "slow"), func() {
	When("creating a collector resource for monitoring with host metrics", func() {
		var (
			ns                  string
			monitoringConfig    *opnimonitoringv1beta1.CollectorConfig
			metricsCollectorObj *opnicorev1beta1.Collector
		)
		It("should succeed in creating the objects", func() {
			ns = makeTestNamespace()
			monitoringConfig = &opnimonitoringv1beta1.CollectorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: otel.MetricsCrdName,
				},
				Spec: opnimonitoringv1beta1.CollectorConfigSpec{
					RemoteWriteEndpoint: "http://test-endpoint",
					PrometheusDiscovery: opnimonitoringv1beta1.PrometheusDiscovery{
						NamespaceSelector: []string{ns},
					},
					OtelSpec: otel.OTELSpec{
						HostMetrics: lo.ToPtr(true),
					},
				},
			}
			metricsCollectorObj = &opnicorev1beta1.Collector{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-monitoring",
				},
				Spec: opnicorev1beta1.CollectorSpec{
					SystemNamespace: ns,
					AgentEndpoint:   "http://test-endpoint",
					MetricsConfig: &corev1.LocalObjectReference{
						Name: otel.MetricsCrdName,
					},
				},
			}

			By("creating the otel collector configs for metrics")
			Expect(k8sClient.Create(context.Background(), monitoringConfig)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), metricsCollectorObj)).To(Succeed())

			By("creating physical objects for discovery")
			for _, obj := range promDiscoveryObejcts(ns) {
				Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
			}
		})

		It("should create the monitoring infra", func() {
			By("checking the agent config map has the hostmetrics & kubelets deps")
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-agent-config", metricsCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveData("receivers.yaml", nil, "config.yaml", nil),
				HaveOwner(metricsCollectorObj),
			))

			By("checking the the aggregator config map embeds scrape configs and selectors")
			By("checking aggregator configmap")
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-aggregator-config", metricsCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveData("aggregator.yaml", nil),
				HaveOwner(metricsCollectorObj),
			))
			By("checking daemonset exists and has hostmetrics requirements")
			Eventually(Object(&appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-collector-agent", metricsCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("otel-collector"),
					HaveVolumeMounts("collector-config"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("ConfigMap"),
					HaveName("collector-config"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("HostPath"),
					HaveName("root"),
				)),
				HaveOwner(metricsCollectorObj),
			))
			By("checking aggregator deployment")
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-collector-aggregator", metricsCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("otel-collector"),
					HaveVolumeMounts("collector-config"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("ConfigMap"),
					HaveName("collector-config"),
				)),
				HaveOwner(metricsCollectorObj),
			))
			By("checking the metrics tls assets secrets exists")
			Eventually(Object(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-otel-tls-assets",
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveOwner(metricsCollectorObj),
			))
		})
		// FIXME: this requires the mock k8s instance to set object endpoints or pod ips
		XIt("should mount required tls assets for service monitors based on prometheus crds", func() {
			Eventually(Object(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-otel-tls-assets",
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveOwner(metricsCollectorObj),
				HaveData(
					"ca.crt", "test-ca",
					"tls.crt", "test-cert",
					"tls.key", "test-key",
				),
			))
		})
	})

	When("creating a collector resource for monitoring without host metrics", func() {
		var (
			ns                  string
			monitoringConfig    *opnimonitoringv1beta1.CollectorConfig
			metricsCollectorObj *opnicorev1beta1.Collector
		)
		It("should succeed in creating the objects", func() {
			ns = makeTestNamespace()
			monitoringConfig = &opnimonitoringv1beta1.CollectorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-monitoring",
				},
				Spec: opnimonitoringv1beta1.CollectorConfigSpec{
					RemoteWriteEndpoint: "http://test-endpoint",
					PrometheusDiscovery: opnimonitoringv1beta1.PrometheusDiscovery{
						NamespaceSelector: []string{ns},
					},
					OtelSpec: otel.OTELSpec{
						HostMetrics: lo.ToPtr(false),
					},
				},
			}
			metricsCollectorObj = &opnicorev1beta1.Collector{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-monitoring2",
				},
				Spec: opnicorev1beta1.CollectorSpec{
					SystemNamespace: ns,
					AgentEndpoint:   "http://test-endpoint",
					MetricsConfig: &corev1.LocalObjectReference{
						Name: "test-monitoring",
					},
				},
			}

			By("creating the otel collector configs for metrics")
			Expect(k8sClient.Create(context.Background(), monitoringConfig)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), metricsCollectorObj)).To(Succeed())

			By("creating physical objects for discovery")
			for _, obj := range promDiscoveryObejcts(ns) {
				Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
			}
		})

		It("should create the monitoring infra", func() {
			By("checking the the aggregator config map embeds scrape configs and selectors")
			By("checking aggregator configmap")
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-aggregator-config", metricsCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveData("aggregator.yaml", nil),
				HaveOwner(metricsCollectorObj),
			))
			By("checking daemonset does not exist")
			Consistently(Object(&appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-collector-agent", metricsCollectorObj.Name),
					Namespace: ns,
				},
			})).ShouldNot(Exist())
			By("checking aggregator deployment")
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-collector-aggregator", metricsCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("otel-collector"),
					HaveVolumeMounts("collector-config"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("ConfigMap"),
					HaveName("collector-config"),
				)),
				HaveOwner(metricsCollectorObj),
			))
			By("checking the metrics tls assets secrets exists")
			Eventually(Object(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-otel-tls-assets",
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveOwner(metricsCollectorObj),
			))
		})
		// FIXME: this requires the mock k8s instance to set object endpoints or pod ips
		XIt("should mount required tls assets for service monitors based on prometheus crds", func() {
			Eventually(Object(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opni-otel-tls-assets",
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveOwner(metricsCollectorObj),
				HaveData(
					"ca.crt", "test-ca",
					"tls.crt", "test-cert",
					"tls.key", "test-key",
				),
			))
		})
	})

	When("creating a collector resource for logging", func() {
		var (
			ns                  string
			loggingConfig       *opniloggingv1beta1.CollectorConfig
			loggingCollectorObj *opnicorev1beta1.Collector
		)
		It("should succeed in creating the objects", func() {
			ns = makeTestNamespace()
			loggingConfig = &opniloggingv1beta1.CollectorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-logging-config",
				},
				Spec: opniloggingv1beta1.CollectorConfigSpec{
					Provider: opniloggingv1beta1.LogProviderGeneric,
				},
			}
			loggingCollectorObj = &opnicorev1beta1.Collector{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-logging",
				},
				Spec: opnicorev1beta1.CollectorSpec{
					SystemNamespace: ns,
					AgentEndpoint:   "http://test-endpoint",
					LoggingConfig: &corev1.LocalObjectReference{
						Name: loggingConfig.Name,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), loggingConfig)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), loggingCollectorObj)).To(Succeed())
		})
		// TODO: handle non happy path cases
		It("should create log scraping infra", func() {
			By("checking agent configmap")
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-agent-config", loggingCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveData("receivers.yaml", nil, "config.yaml", nil),
				HaveOwner(loggingCollectorObj),
			))
			By("checking aggregator configmap")
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-aggregator-config", loggingCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveData("aggregator.yaml", nil),
				HaveOwner(loggingCollectorObj),
			))
			By("checking daemonset")
			Eventually(Object(&appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-collector-agent", loggingCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("otel-collector"),
					HaveVolumeMounts("collector-config", "varlogpods"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("ConfigMap"),
					HaveName("collector-config"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("HostPath"),
					HaveName("varlogpods"),
				)),
				HaveOwner(loggingCollectorObj),
			))
			By("checking deployment")
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-collector-aggregator", loggingCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("otel-collector"),
					HaveVolumeMounts("collector-config"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("ConfigMap"),
					HaveName("collector-config"),
				)),
				HaveOwner(loggingCollectorObj),
			))
		})
	})

	When("creating a collector resource for traces", func() {
		var (
			ns                string
			traceConfig       *opniloggingv1beta1.CollectorConfig
			traceCollectorObj *opnicorev1beta1.Collector
		)
		It("should succeed in creating the objects", func() {
			ns = makeTestNamespace()
			traceConfig = &opniloggingv1beta1.CollectorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-trace-config",
				},
				Spec: opniloggingv1beta1.CollectorConfigSpec{
					Provider: opniloggingv1beta1.LogProviderGeneric,
				},
			}
			traceCollectorObj = &opnicorev1beta1.Collector{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-trace",
				},
				Spec: opnicorev1beta1.CollectorSpec{
					SystemNamespace: ns,
					AgentEndpoint:   "http://test-endpoint",
					TracesConfig: &corev1.LocalObjectReference{
						Name: traceConfig.Name,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), traceConfig)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), traceCollectorObj)).To(Succeed())
		})

		It("should create aggregator with trace capabilities", func() {
			By("checking agent configmap")
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-agent-config", traceCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveData("receivers.yaml", nil, "config.yaml", nil),
				HaveOwner(traceCollectorObj),
			))
			By("checking aggregator configmap")
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-aggregator-config", traceCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveData("aggregator.yaml", nil),
				HaveOwner(traceCollectorObj),
			))
			By("checking deployment")
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-collector-aggregator", traceCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("otel-collector"),
					HaveVolumeMounts("collector-config"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("ConfigMap"),
					HaveName("collector-config"),
				)),
				HaveOwner(traceCollectorObj),
			))
		})

		It("should not create collector agents", func() {
			By("checking daemonset")
			Eventually(Object(&appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-collector-agent", traceCollectorObj.Name),
					Namespace: ns,
				},
			})).Should(Not(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("otel-collector"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("ConfigMap"),
					HaveName("collector-config"),
				)),
			)))
		})
	})
})

func promDiscoveryObejcts(ns string) []client.Object {

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "some-service",
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/instance": "opni",
				"app.kubernetes.io/name":     "kube-state-metrics",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"test": "A",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}

	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: ns,
					Labels: map[string]string{
						"test": "A",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "idk",
							Image: "nginx",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
									Protocol:      "TCP",
								},
							},
						},
					},
				},
			},
		},
	}

	podMonitor := &promoperatorv1.PodMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-kube-state-metrics",
			Namespace: ns,
		},
		Spec: promoperatorv1.PodMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"test": "A",
				},
			},
			PodMetricsEndpoints: []promoperatorv1.PodMetricsEndpoint{
				{
					Port: "http",
				},
			},
		},
	}

	secretName := "test-tls-secret"
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("test-ca"),
			"tls.crt": []byte("test-cert"),
			"tls.key": []byte("test-key"),
		},
	}

	service2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-service",
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/instance": "opni",
				"app.kubernetes.io/name":     "kube-state-metrics",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"test": "tls",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http-metrics",
					Port:       8080,
					TargetPort: intstr.FromString("http-metrics"),
				},
			},
		},
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-deployment",
			Namespace: ns,
			Labels: map[string]string{
				//TODO
				"test": "tls",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"test": "tls",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-pod",
					Namespace: ns,
					Labels: map[string]string{
						"test": "tls",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "tls-container",
							Image: "nginx",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http-metrics",
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		},
	}

	serviceMonitorWithSecretTLS := &promoperatorv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-service-monitor",
			Namespace: ns,
		},
		Spec: promoperatorv1.ServiceMonitorSpec{
			Endpoints: []promoperatorv1.Endpoint{
				{
					Path:   "/metrics",
					Port:   "http-metrics",
					Scheme: "https",
					TLSConfig: &promoperatorv1.TLSConfig{
						SafeTLSConfig: promoperatorv1.SafeTLSConfig{
							CA: promoperatorv1.SecretOrConfigMap{
								Secret: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: secretName,
									},
									Key: "ca.crt",
								},
							},
							Cert: promoperatorv1.SecretOrConfigMap{
								Secret: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: secretName,
									},
									Key: "tls.crt",
								},
							},
							KeySecret: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: secretName,
								},
								Key: "tls.key",
							},
						},
					},
				},
			},
			NamespaceSelector: promoperatorv1.NamespaceSelector{
				MatchNames: []string{ns},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"test": "tls",
				},
			},
		},
	}
	serviceMonitor := &promoperatorv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-kube-state-metrics",
			Namespace: ns,
		},
		Spec: promoperatorv1.ServiceMonitorSpec{
			Endpoints: []promoperatorv1.Endpoint{
				{
					HonorLabels: true,
					Port:        "http",
				},
			},
			JobLabel: "app.kubernetes.io/name",
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/instance": "opni",
					"app.kubernetes.io/name":     "kube-state-metrics",
				},
			},
		},
	}
	res := []client.Object{}
	for _, pod := range pods.Items {
		res = append(res, &pod)
	}

	return append(res,
		[]client.Object{
			service,
			tlsSecret,
			service2,
			deploy,
			podMonitor,
			serviceMonitor,
			serviceMonitorWithSecretTLS}...,
	)
}
