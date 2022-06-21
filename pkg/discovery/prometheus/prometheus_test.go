package prometheus_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	pdiscovery "github.com/rancher/opni/pkg/discovery/prometheus"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/notifier"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Prometheus Service Discovery", Ordered, Label(test.Unit, test.Slow), func() {
	testServiceMonitorGroup1 := []monitoringv1.ServiceMonitor{
		{
			Spec: monitoringv1.ServiceMonitorSpec{
				JobLabel:  "Foo",
				Endpoints: []monitoringv1.Endpoint{},
			},
		},
		{
			Spec: monitoringv1.ServiceMonitorSpec{
				JobLabel:  "Bar",
				Endpoints: []monitoringv1.Endpoint{},
			},
		},
		{
			Spec: monitoringv1.ServiceMonitorSpec{
				JobLabel:  "Baz",
				Endpoints: []monitoringv1.Endpoint{},
			},
		},
	}

	testServiceMonitorGroup2 := make([]monitoringv1.ServiceMonitor, len(testServiceMonitorGroup1))
	for i, group := range testServiceMonitorGroup1 {
		testServiceMonitorGroup2[i] = *group.DeepCopy()
	}

	testPodMonitorGroup1 := []monitoringv1.PodMonitor{
		{
			Spec: monitoringv1.PodMonitorSpec{
				JobLabel:            "Foo",
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{},
			},
		},
		{
			Spec: monitoringv1.PodMonitorSpec{
				JobLabel:            "Bar",
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{},
			},
		},
		{
			Spec: monitoringv1.PodMonitorSpec{
				JobLabel:            "Baz",
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{},
			},
		},
	}

	var k8sClient client.Client
	var finder notifier.Finder[pdiscovery.PrometheusService]
	BeforeAll(func() {
		env := test.Environment{
			TestBin:           "../../../testbin/bin",
			CRDDirectoryPaths: []string{"testdata/crds"},
		}
		GinkgoWriter.Write([]byte(env.CRDDirectoryPaths[0]))
		restConfig, err := env.StartK8s()
		Expect(err).ToNot(HaveOccurred())

		scheme := runtime.NewScheme()
		util.Must(clientgoscheme.AddToScheme(scheme))
		util.Must(monitoringv1.AddToScheme(scheme))
		k8sClient, err = client.New(restConfig, client.Options{
			Scheme: scheme,
		})

		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(env.Stop)
		Expect(k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "namespace1",
			},
		})).To(Succeed())

		Expect(k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "namespace2",
			},
		})).To(Succeed())

		finder = pdiscovery.NewPrometheusServiceFinder(k8sClient)
	})

	It("Should initially find no monitors", func() {
		monitors, err := finder.Find(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(monitors).To(BeEmpty())
	})

	When("Creating ServiceMonitors", func() {
		It("should find the created monitors", func() {
			for i, svcm := range testServiceMonitorGroup1 {
				Expect(k8sClient.Create(context.Background(), &monitoringv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test%d", i),
						Namespace: "namespace1",
					},
					Spec: svcm.Spec,
				})).To(Succeed())
			}

			By("Finding the newly declared service monitors")
			monitors, err := finder.Find(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(monitors).To(HaveLen(4))
			Expect([]string{
				monitors[0].GetServiceName(),
				monitors[1].GetServiceName(),
				monitors[2].GetServiceName(),
			}).To(ContainElements("test0", "test1", "test2"))

			Expect([]string{
				monitors[0].GetServiceId(),
				monitors[1].GetServiceId(),
				monitors[2].GetServiceId(),
			}).To(ContainElements("Foo", "Baz", "Bar"))

			for i, svcm := range testServiceMonitorGroup1 {
				Expect(k8sClient.Delete(context.Background(), &monitoringv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test%d", i),
						Namespace: "namespace1",
					},
					Spec: svcm.Spec,
				})).To(Succeed())
			}
		})
	})

	When("Creating Service Monitors in different namespaces", func() {
		It("should find the created monitors", func() {

			for i, svcm := range testServiceMonitorGroup2 {
				Expect(k8sClient.Create(context.Background(), &monitoringv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test%d", i),
						Namespace: "namespace2",
					},
					Spec: svcm.Spec,
				})).To(Succeed())
			}

			By("Finding the newly declared service monitors")
			monitors, err := finder.Find(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(monitors).To(HaveLen(6))
			Expect([]string{
				monitors[0].GetServiceName(),
				monitors[1].GetServiceName(),
				monitors[2].GetServiceName(),
				monitors[3].GetServiceName(),
				monitors[4].GetServiceName(),
				monitors[5].GetServiceName(),
			}).To(ContainElements("test0", "test1", "test2", "test0", "test1", "test2"))

			Expect([]string{
				monitors[0].GetServiceId(),
				monitors[1].GetServiceId(),
				monitors[2].GetServiceId(),
				monitors[3].GetServiceId(),
				monitors[4].GetServiceId(),
				monitors[5].GetServiceId(),
			}).To(ContainElements("Foo", "Baz", "Bar", "Foo", "Baz", "Bar"))

			for i, svcm := range testServiceMonitorGroup1 {
				Expect(k8sClient.Delete(context.Background(), &monitoringv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test%d", i),
						Namespace: "namespace1",
					},
					Spec: svcm.Spec,
				})).To(Succeed())
			}

			for i, svcm := range testServiceMonitorGroup2 {
				Expect(k8sClient.Delete(context.Background(), &monitoringv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test%d", i),
						Namespace: "namespace1",
					},
					Spec: svcm.Spec,
				})).To(Succeed())
			}
		})
	})
	When("Creating PodMonitors", func() {
		It("should find the created monitors", func() {
			for i, svcm := range testPodMonitorGroup1 {
				Expect(k8sClient.Create(context.Background(), &monitoringv1.PodMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test%d", i),
						Namespace: "namespace1",
					},
					Spec: svcm.Spec,
				})).To(Succeed())
			}

			By("Finding the newly declared service monitors")
			monitors, err := finder.Find(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(monitors).To(HaveLen(3))
			Expect([]string{
				monitors[0].GetServiceName(),
				monitors[1].GetServiceName(),
				monitors[2].GetServiceName(),
			}).To(ContainElements("test0", "test1", "test2"))

			Expect([]string{
				monitors[0].GetServiceId(),
				monitors[1].GetServiceId(),
				monitors[2].GetServiceId(),
			}).To(ContainElements("Foo", "Baz", "Bar"))

			for i, svcm := range testPodMonitorGroup1 {
				Expect(k8sClient.Delete(context.Background(), &monitoringv1.PodMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test%d", i),
						Namespace: "namespace1",
					},
					Spec: svcm.Spec,
				})).To(Succeed())
			}
		})
	})
})
