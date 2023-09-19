package metrics_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/slo/backend/metrics"
	"github.com/samber/lo"
)

var _ = Describe("Windows test", Label("unit"), func() {
	DescribeTable("invalid Window struct", func(window metrics.Window) {
		Expect(window.Validate()).NotTo(Succeed())
	},
		Entry("Empty window", metrics.Window{}),
		Entry("missing long window", metrics.Window{
			ShortWindow:        model.Duration(time.Minute),
			ErrorBudgetPercent: 2,
		}),
		Entry("missing short window", metrics.Window{
			LongWindow:         model.Duration(time.Hour),
			ErrorBudgetPercent: 2,
		}),
		Entry("missing error budget", metrics.Window{
			LongWindow:  model.Duration(time.Hour),
			ShortWindow: model.Duration(time.Minute),
		}),
	)

	DescribeTable("valid windows -- duration strings", func(window *metrics.MWMBWindows, windowRange []string) {
		Expect(window.Validate()).To(Succeed())
		wr := window.WindowRange()
		periodStrs := lo.Map(wr, func(w metrics.WindowMetadata, _ int) string {
			return w.WindowDur.String()
		})
		Expect(periodStrs).To(Equal(windowRange))
	},
		Entry("default window", metrics.WindowDefaults(
			model.Duration(time.Hour*24*30),
		), []string{"5m", "30m", "1h", "2h", "6h", "6h", "1d", "3d", "30d"}),
		Entry("custom window", metrics.GenerateMWMBWindows(
			model.Duration(time.Hour*24*28),
			model.Duration(time.Minute*5),
		), []string{"5m", "30m", "1h", "2h", "6h", "6h", "1d", "3d", "4w"}),
		Entry("custom window short", metrics.GenerateMWMBWindows(
			model.Duration(time.Hour*5),
			model.Duration(time.Second*5),
		), []string{"5s", "30s", "1m", "2m", "6m", "6m", "24m", "1h12m", "5h"}),
		Entry("normalizing period interval",
			metrics.GenerateMWMBWindows(
				model.Duration(time.Minute*72*10),
				model.Duration(metrics.NormalizePeriodToBudgetingInterval(time.Minute*72*10)),
			), []string{"5s", "30s", "1m", "2m", "6m", "6m", "24m", "1h12m", "12h"}),
		Entry("custom window very short", metrics.GenerateMWMBWindows(
			model.Duration(time.Hour),
			model.Duration(time.Second),
		), []string{"1s", "6s", "12s", "24s", "1m12s", "1m12s", "4m48s", "14m24s", "1h"}),
		Entry("normalizing very short interval",
			metrics.GenerateMWMBWindows(
				model.Duration(aVeryExactDuration),
				model.Duration(metrics.NormalizePeriodToBudgetingInterval(aVeryExactDuration)),
			), []string{"1s", "6s", "12s", "24s", "1m12s", "1m12s", "4m48s", "14m24s", "2h24m"},
		),
	)
	// TODO : burn rate factor tests
})

var (
	aVeryExactDuration = (time.Minute*14 + (time.Second * 24)) * 10
)
