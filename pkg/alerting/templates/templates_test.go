package templates_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/templates"
)

var _ = Describe("Templates Suite", func() {
	When("We convert strings to their template values", func() {
		It("Should convert singular values", func() {
			Expect(templates.StrAsTemplateValue("foo")).To(Equal("{{ .foo }}"))
		})

		It("Should convert slices as their template values", func() {
			Expect(templates.StrSliceAsTemplates([]string{"foo", "bar"})).To(Equal([]string{"{{ .foo }}", "{{ .bar }}"}))
		})
	})
})
