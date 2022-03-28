package resources_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
)

var _ = Describe("Labels", Label("unit"), func() {
	var labels resources.OpensearchLabels
	When("creating an instance of OpensearchLabels", func() {
		It("should contain only the app label", func() {
			labels = resources.NewOpensearchLabels()
			Expect(labels).To(BeEquivalentTo(map[string]string{
				"app": "opensearch",
			}))
		})
	})
	When("adding a role", func() {
		It("should contain the role label", func() {
			labelsWithRole := labels.WithRole(v1beta2.OpensearchClientRole)
			Expect(labelsWithRole).To(BeEquivalentTo(map[string]string{
				"app":  "opensearch",
				"role": "client",
			}))
			Expect(labelsWithRole.Role()).To(BeEquivalentTo(v1beta2.OpensearchClientRole))
		})
		It("should not mutate the original instance", func() {
			Expect(labels).To(BeEquivalentTo(map[string]string{
				"app": "opensearch",
			}))
		})
	})
	When("a role already exists", func() {
		It("should replace it", func() {
			labelsWithRole := labels.WithRole(v1beta2.OpensearchClientRole)
			Expect(labelsWithRole).To(BeEquivalentTo(map[string]string{
				"app":  "opensearch",
				"role": "client",
			}))
			Expect(labelsWithRole.Role()).To(BeEquivalentTo(v1beta2.OpensearchClientRole))
			labelsWithRole = labelsWithRole.WithRole(v1beta2.OpensearchDataRole)
			Expect(labelsWithRole).To(BeEquivalentTo(map[string]string{
				"app":  "opensearch",
				"role": "data",
			}))
			Expect(labelsWithRole.Role()).To(BeEquivalentTo(v1beta2.OpensearchDataRole))
		})
	})
})
