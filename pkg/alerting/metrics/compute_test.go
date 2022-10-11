package metrics_test

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/metrics"
)

var _ = Describe("compute alerting options pipeline & compute alerts construction", func() {
	When("users want to create cpu compute alerts", func() {
		Specify("alerting/metrics package should export cpu alerts", func() {
			_, templateOk := metrics.ComputeNameToTemplate["cpu"]
			_, optionsOk := metrics.ComputeNameToOpts["cpu"]
			Expect((templateOk && optionsOk)).To(BeTrue())
		})

		Specify("cpu alerts should have parsed an available template", func() {
			tmpl := metrics.ComputeNameToTemplate["cpu"]
			Expect(tmpl).NotTo(BeNil())
			definedTmpls := tmpl.Templates()
			Expect(definedTmpls).NotTo(HaveLen(0))
		})

		Specify("The template should be executed from options", func() {
			tmpl := metrics.ComputeNameToTemplate["cpu"]
			opts := metrics.ComputeNameToOpts["cpu"]
			var b bytes.Buffer
			err := tmpl.Execute(&b, opts)
			Expect(err).NotTo(HaveOccurred())
		})

		Specify("valid inputs should construct valid promQL", func() {
			validInputs := []metrics.CpuRuleOptions{
				{
					Node: []string{""},
				},
			}

			for _, input := range validInputs {
				tmpl := metrics.ComputeNameToTemplate["cpu"]
				var b bytes.Buffer
				err := tmpl.Execute(&b, input)
				Expect(err).NotTo(HaveOccurred())
				Expect(b.String()).NotTo(Equal(""))
			}
		})

	})
})
