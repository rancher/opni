package metrics_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
)

var _ = Describe("Constructing cortex alerting rules", func() {
	It("should be able to construct a basic cortex alerting rule", func() {
		simple := metrics.AlertingRule{
			Alert: "valid-rule-name",
			Expr:  "up == 1",
		}
		Expect(simple.Validate()).To(Succeed())
	})

	It("Should be able to compose simple rules together", func() {
		simple := metrics.AlertingRule{
			Alert: "valid-rule-name",
			Expr:  "up == 1",
		}
		Expect(simple.Validate()).To(Succeed())
		simple2 := metrics.AlertingRule{
			Alert: "valid-rule-name2",
			Expr:  "up == 0",
		}
		Expect(simple2.Validate()).To(Succeed())
		buildAndId := shared.NewAlertingRefId()

		promRule, err := simple.And(&simple2).Build(buildAndId)
		Expect(err).To(Succeed())
		Expect(promRule.Alert).To(Equal(buildAndId))
		Expect(promRule.Expr).To(Equal(fmt.Sprintf("(%s) and (%s)", simple.Alert, simple2.Alert)))

		buildOrId := shared.NewAlertingRefId()
		promRule, err = simple.Or(&simple2).Build(buildOrId)
		Expect(err).To(Succeed())
		Expect(promRule.Alert).To(Equal(buildOrId))
		Expect(promRule.Expr).To(Equal(fmt.Sprintf("(%s) or (%s)", simple.Alert, simple2.Alert)))
	})

	It("Should have the control flow composition disabled", func() {
		simple := metrics.AlertingRule{
			Alert: "valid-rule-name",
			Expr:  "up == 1",
		}
		Expect(simple.Validate()).To(Succeed())
		simple2 := metrics.AlertingRule{
			Alert: "valid-rule-name2",
			Expr:  "up == 0",
		}
		Expect(simple2.Validate()).To(Succeed())
		res := simple.IfForSecondsThen(&simple2, time.Second*0)
		Expect(res).To(BeNil())

		res2 := simple.IfNotForSecondsThen(&simple2, time.Second*0)
		Expect(res2).To(BeNil())
	})
})
