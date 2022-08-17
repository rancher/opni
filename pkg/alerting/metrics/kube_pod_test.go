package metrics_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/metrics"
)

var _ = Describe("Building Kube Pod State Alert Rules", func() {
	It("Should be able to construct a basic kube pod state alert rule", func() {
		alertRule, err := metrics.NewKubePodStateRule(
			"opni-alerting-0",
			"",
			metrics.KubePodStates[0],
			"60s",
			nil,
			nil)
		Expect(err).To(Succeed())
		_, err = alertRule.Build(uuid.New().String())
		Expect(err).To(Succeed())
	})
})
