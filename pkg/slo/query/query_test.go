package query_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/slo/query"
	"github.com/rancher/opni/pkg/test"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

var _ = Describe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label(test.Unit, test.Slow), func() {
	// ctx := context.Background()

	validMetricQuery := func(query query.MetricQuery) error {
		return nil
	}

	When("We define pre-configured metrics in def.go", func() {
		It("Should produce valid templates and regex at compile time", func() {
			Expect(query.AvailableQueries).To(HaveLen(3))
		})

		It("Query implementations should implement the interfaces required", func() {
			q := &query.PrometheusQueryImpl{}
			validMetricQuery(q)
		})
	})

	When("We use matchers", func() {
		It("Should use a MatchMinLength to match the shortest metric", func() {
			m := query.MatchMinLength
			discoveredMetrics := []string{
				"alertmanager_http_request_duration_seconds_bucket",
				"alertmanager_http_request_duration_seconds_count",
				"alertmanager_http_request_duration_seconds_sum",
			}
			Expect(m(discoveredMetrics)).To(Equal("alertmanager_http_request_duration_seconds_sum"))
		})
	})

	When("We construct metrics via Service defintions", func() {
		It("Should fill in the templates & return a response struct", func() {
			uptime, ok := query.AvailableQueries["uptime"]
			Expect(ok).To(BeTrue())
			service := &api.Service{
				JobId:         "prometheus",
				MetricName:    "uptime",
				MetricIdGood:  "up",
				MetricIdTotal: "up",
				ClusterId:     "agent",
			}
			resp, err := uptime.Construct(service)
			Expect(err).To(Succeed())
			Expect(resp).To(Not(BeNil()))
			Expect(resp.GoodQuery).To(Not(BeNil()))
			Expect(resp.TotalQuery).To(Not(BeNil()))
			Expect(resp.GoodQuery).To(Equal("sum(rate(up{job=\"prometheus\", up=1}[{{.window}}]))"))
			Expect(resp.TotalQuery).To(Equal("sum(rate(up{job=\"prometheus\"}[{{.window}}]))"))

		})
	})
})
