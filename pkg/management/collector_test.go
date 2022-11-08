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
			Eventually(descs).Should(HaveLen(4))
			Consistently(descs).Should(HaveLen(4))
			metrics := make(chan prometheus.Metric, 100)
			tv.ifaces.collector.Collect(metrics)
			Consistently(metrics).Should(BeEmpty())
		})
	})
	When("clusters are present", func() {
		It("should collect metrics for each cluster", func() {
			tv.storageBackend.CreateCluster(context.Background(), &corev1.Cluster{
				Id: "cluster-1",
				Metadata: &corev1.ClusterMetadata{
					Labels:       map[string]string{corev1.NameLabel: "cluster-1"},
					Capabilities: []*corev1.ClusterCapability{{Name: "test"}},
				},
			})
			tv.storageBackend.CreateCluster(context.Background(), &corev1.Cluster{
				Id: "cluster-2",
				Metadata: &corev1.ClusterMetadata{
					Labels:       map[string]string{corev1.NameLabel: "cluster-2"},
					Capabilities: []*corev1.ClusterCapability{{Name: "test"}},
				},
			})

			c := make(chan *prometheus.Desc, 100)
			tv.ifaces.collector.Describe(c)
			Expect(c).To(HaveLen(4))
			descs := make([]string, 0, 4)
			for i := 0; i < 4; i++ {
				descs = append(descs, (<-c).String())
			}
			Expect(descs).To(ConsistOf(
				descriptorString(
					"opni_cluster_info",
					"Cluster information",
					[]string{},
					[]string{"cluster_id", "friendly_name"},
				),
				descriptorString(
					"opni_agent_up",
					"Agent connection status",
					[]string{},
					[]string{"cluster_id"},
				),
				descriptorString(
					"opni_agent_ready",
					"Agent readiness status",
					[]string{},
					[]string{"cluster_id", "conditions"},
				),
				descriptorString(
					"opni_agent_status_summary",
					"Agent status summary",
					[]string{},
					[]string{"cluster_id", "summary"},
				),
			))

			metrics := make(chan prometheus.Metric, 100)
			tv.ifaces.collector.Collect(metrics)
			Expect(metrics).To(Receive())
			Expect(metrics).To(Receive())
		})
	})
})
