package metrics

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	corev1 "k8s.io/api/core/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const testNamespace = "test-ns"

var _ = Describe("Target Runner", Ordered, Label("integration"), func() {
	var (
		env test.Environment

		kubeClient kubernetes.Interface
		promClient monitoringclient.Interface
		discoverer *agent.PrometheusDiscoverer
	)

	BeforeAll(func() {
		lg := logger.NewPluginLogger().Named("test-discvoery")

		env = test.Environment{
			TestBin: "../../../testbin/bin",
			CRDDirectoryPaths: []string{
				"../../../config/crd/prometheus",
			},
		}

		config, _, err := env.StartK8s()
		Expect(err).NotTo(HaveOccurred())

		kubeClient, err = kubernetes.NewForConfig(config)
		Expect(err).NotTo(HaveOccurred())

		_, err = kubeClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
			ObjectMeta: apimetav1.ObjectMeta{
				Name: testNamespace,
			},
			Spec: corev1.NamespaceSpec{},
		}, apimetav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		promClient, err = monitoringclient.NewForConfig(config)
		Expect(err).NotTo(HaveOccurred())

		discoverer, err = agent.NewPrometheusDiscoverer(agent.DiscovererConfig{
			RESTConfig: config,
			Context:    context.Background(),
			Logger:     lg,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	When("no prometheuses exist", func() {
		It("find not prometheuses", func() {
			entries, err := discoverer.Discover()
			Expect(err).NotTo(HaveOccurred())

			Expect(len(entries)).To(Equal(0))
		})
	})

	When("some prometheuses exist", func() {
		It("can retrieve from all namespaces", func() {
			_, err := promClient.MonitoringV1().Prometheuses("default").Create(context.Background(), &monitoringv1.Prometheus{
				ObjectMeta: apimetav1.ObjectMeta{
					Name: "test-prometheus",
				},
				Spec: monitoringv1.PrometheusSpec{
					CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
						ExternalURL: "external-url.domain",
					},
				},
			}, apimetav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = promClient.MonitoringV1().Prometheuses(testNamespace).Create(context.Background(), &monitoringv1.Prometheus{
				ObjectMeta: apimetav1.ObjectMeta{
					Name: "test-prometheus",
				},
				Spec: monitoringv1.PrometheusSpec{
					CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
						ExternalURL: "external-url.domain",
					},
				},
			}, apimetav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			entries, err := discoverer.Discover()
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(ContainElements(
				&remoteread.DiscoveryEntry{
					Name:             "test-prometheus",
					ClusterId:        "",
					ExternalEndpoint: "external-url.domain",
					InternalEndpoint: "test-prometheus.default.svc.cluster.local",
				},
				&remoteread.DiscoveryEntry{
					Name:             "test-prometheus",
					ClusterId:        "",
					ExternalEndpoint: "external-url.domain",
					InternalEndpoint: "test-prometheus.test-ns.svc.cluster.local",
				},
			))

			entries, err = discoverer.DiscoverIn("default")
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(ContainElement(
				&remoteread.DiscoveryEntry{
					Name:             "test-prometheus",
					ClusterId:        "",
					ExternalEndpoint: "external-url.domain",
					InternalEndpoint: "test-prometheus.default.svc.cluster.local",
				}))

			entries, err = discoverer.DiscoverIn(testNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(ContainElement(&remoteread.DiscoveryEntry{
				Name:             "test-prometheus",
				ClusterId:        "",
				ExternalEndpoint: "external-url.domain",
				InternalEndpoint: "test-prometheus.test-ns.svc.cluster.local",
			}))
		})
	})
})
