package management_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins"
)

func descriptorString(fqName, help string, constLabels, varLabels []string) string {
	return fmt.Sprintf(
		"Desc{fqName: %q, help: %q, constLabels: {%s}, variableLabels: %v}",
		fqName,
		help,
		strings.Join(constLabels, ","),
		varLabels,
	)
}

var _ = Describe("Collector", Ordered, Label("slow"), func() {
	var tv *testVars
	BeforeAll(setupManagementServer(&tv, plugins.NoopLoader))

	When("no clusters are present", func() {
		It("should collect descriptors but no metrics", func() {
			descs := make(chan *prometheus.Desc, 100)
			tv.ifaces.collector.Describe(descs)
			Eventually(descs).Should(Receive(WithTransform(fmt.Stringer.String, Equal(
				descriptorString(
					"opni_monitoring_cluster_info",
					"Cluster information",
					[]string{},
					[]string{"cluster_id", "friendly_name"},
				),
			))))
			Consistently(descs).ShouldNot(Receive())
			metrics := make(chan prometheus.Metric, 100)
			tv.ifaces.collector.Collect(metrics)
			Consistently(metrics).ShouldNot(Receive())
		})
	})
	When("clusters are present", func() {
		It("should collect metrics for each cluster", func() {
			tv.storageBackend.CreateCluster(context.Background(), &corev1.Cluster{
				Id: "cluster-1",
				Metadata: &corev1.ClusterMetadata{
					Labels:       map[string]string{"kubernetes.io/metadata.name": "cluster-1"},
					Capabilities: []*corev1.ClusterCapability{{Name: "test"}},
				},
			})
			tv.storageBackend.CreateCluster(context.Background(), &corev1.Cluster{
				Id: "cluster-2",
				Metadata: &corev1.ClusterMetadata{
					Labels:       map[string]string{"kubernetes.io/metadata.name": "cluster-2"},
					Capabilities: []*corev1.ClusterCapability{{Name: "test"}},
				},
			})

			descs := make(chan *prometheus.Desc, 100)
			tv.ifaces.collector.Describe(descs)
			Expect(descs).To(Receive(WithTransform(fmt.Stringer.String, Equal(
				descriptorString(
					"opni_monitoring_cluster_info",
					"Cluster information",
					[]string{},
					[]string{"cluster_id", "friendly_name"},
				),
			))))

			metrics := make(chan prometheus.Metric, 100)
			tv.ifaces.collector.Collect(metrics)
			Expect(metrics).To(Receive())
			Expect(metrics).To(Receive())
		})
	})
})
