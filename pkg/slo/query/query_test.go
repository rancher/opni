package query_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/slo/query"
	api "github.com/rancher/opni/plugins/slo/apis/slo"
)

func constructionShouldSucceed(q *query.SLOQueryResult, err error) {
	Expect(err).To(Succeed())
	Expect(q).To(Not(BeNil()))
	Expect(q.GoodQuery).To(Not(BeNil()))
	Expect(q.TotalQuery).To(Not(BeNil()))
}

var _ = Describe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label("unit", "slow"), func() {
	// ctx := context.Background()

	validMetricQuery := func(query query.MetricQuery) error {
		return nil
	}

	When("We define pre-configured metrics in def.go", func() {
		It("Should produce valid templates and regex at compile time", func() {
			Expect(query.AvailableQueries).ToNot(HaveLen(0)) // > 0 available pre configured metrics
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
			service := &api.ServiceInfo{
				JobId:         "prometheus",
				MetricName:    "uptime",
				MetricIdGood:  "up",
				MetricIdTotal: "up",
				ClusterId:     "agent",
			}
			respUp, err := uptime.Construct(service)
			constructionShouldSucceed(respUp, err)
			Expect(respUp.GoodQuery).To(Equal("(sum(rate(up{job=\"prometheus\"} == 1)))[{{.window}}]"))
			Expect(respUp.TotalQuery).To(Equal("(sum(rate(up{job=\"prometheus\"})))[{{.window}}]"))

			latency, ok := query.AvailableQueries["http-latency"]
			Expect(ok).To(BeTrue())
			respLa, err := latency.Construct(service)
			constructionShouldSucceed(respLa, err)

			availability, ok := query.AvailableQueries["http-availability"]
			Expect(ok).To(BeTrue())
			respAv, err := availability.Construct(service)
			constructionShouldSucceed(respAv, err)

		})
	})
})
