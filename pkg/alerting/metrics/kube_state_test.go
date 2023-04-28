package metrics_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
)

var _ = Describe("Building Kube Pod State Alert Rules", Label("unit"), func() {
	It("Should be able to construct a basic kube pod state alert rule", func() {
		alertRule, err := metrics.NewKubeStateRule(
			"pod",
			"test",
			"default",
			"Running",
			"1m",
			nil,
		)
		Expect(err).To(Succeed())
		_, err = alertRule.Build(shared.NewAlertingRefId())
		Expect(err).To(Succeed())
	})
})
