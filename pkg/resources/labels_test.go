package resources_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
)

var _ = Describe("Labels", func() {
	var labels resources.ElasticLabels
	When("creating an instance of ElasticLabels", func() {
		It("should contain only the app label", func() {
			labels = resources.NewElasticLabels()
			Expect(labels).To(BeEquivalentTo(map[string]string{
				"app": "opendistro-es",
			}))
		})
	})
	When("adding a role", func() {
		It("should contain the role label", func() {
			labelsWithRole := labels.WithRole(v1beta1.ElasticClientRole)
			Expect(labelsWithRole).To(BeEquivalentTo(map[string]string{
				"app":  "opendistro-es",
				"role": "client",
			}))
			Expect(labelsWithRole.Role()).To(BeEquivalentTo(v1beta1.ElasticClientRole))
		})
		It("should not mutate the original instance", func() {
			Expect(labels).To(BeEquivalentTo(map[string]string{
				"app": "opendistro-es",
			}))
		})
	})
	When("a role already exists", func() {
		It("should replace it", func() {
			labelsWithRole := labels.WithRole(v1beta1.ElasticClientRole)
			Expect(labelsWithRole).To(BeEquivalentTo(map[string]string{
				"app":  "opendistro-es",
				"role": "client",
			}))
			Expect(labelsWithRole.Role()).To(BeEquivalentTo(v1beta1.ElasticClientRole))
			labelsWithRole = labelsWithRole.WithRole(v1beta1.ElasticDataRole)
			Expect(labelsWithRole).To(BeEquivalentTo(map[string]string{
				"app":  "opendistro-es",
				"role": "data",
			}))
			Expect(labelsWithRole.Role()).To(BeEquivalentTo(v1beta1.ElasticDataRole))
		})
	})
})
