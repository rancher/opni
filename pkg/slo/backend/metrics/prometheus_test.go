package metrics_test

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	promql "github.com/prometheus/prometheus/promql/parser"
	"github.com/rancher/opni/pkg/slo/backend"
	"github.com/rancher/opni/pkg/slo/backend/metrics"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
)

var _ = Describe("Prometheus SLO", Label("unit"), func() {
	When("We use metric generators", func() {
		Specify("constant generators should produce valid promQL", func() {
			cnst := metrics.NewConstantMetricGenerator("hello_vector", 0, 9, map[string]string{
				"foo ": "bar",
			})

			Expect(cnst.Id()).To(Equal("hello_vector"))
			Expect(cnst.Expr()).To(Equal("vector(0.000000000)"))

			_, err := promql.ParseExpr(cnst.Expr())
			Expect(err).NotTo(HaveOccurred())

			rule, err := cnst.Rule()
			Expect(err).NotTo(HaveOccurred())
			Expect(rule.Record.Value).To(Equal(cnst.Id()))
			Expect(rule.Expr.Value).To(Equal(cnst.Expr()))
			Expect(rule.Labels).To(Equal(map[string]string{
				"foo ": "bar",
			}))
		})

		Specify("sum rate generators should produce valid promQL", func() {
			sr := metrics.NewSumRateGenerator("hello_vector", metrics.WindowMetadata{WindowDur: prommodel.Duration(time.Minute)})

			// this is a partial generator
			Expect(sr.Id()).To(Equal(""))
			Expect(sr.Expr()).To(Equal("sum(rate(hello_vector[1m]))"))
			_, err := sr.Rule()
			Expect(err).To(HaveOccurred())

			_, err = promql.ParseExpr(sr.Expr())
			Expect(err).NotTo(HaveOccurred())

			seconds := metrics.NewSumRateGenerator("hello_vector", metrics.WindowMetadata{WindowDur: prommodel.Duration(time.Second)})
			Expect(seconds.Id()).To(Equal(""))
			Expect(seconds.Expr()).To(Equal("sum(rate(hello_vector[1s]))"))

			_, err = seconds.Rule()
			Expect(err).To(HaveOccurred())

			_, err = promql.ParseExpr(seconds.Expr())
			Expect(err).NotTo(HaveOccurred())

			hours := metrics.NewSumRateGenerator("hello_vector", metrics.WindowMetadata{WindowDur: prommodel.Duration(time.Hour)})
			Expect(hours.Id()).To(Equal(""))
			Expect(hours.Expr()).To(Equal("sum(rate(hello_vector[1h]))"))

			_, err = hours.Rule()
			Expect(err).To(HaveOccurred())

			days := metrics.NewSumRateGenerator("hello_vector", metrics.WindowMetadata{WindowDur: prommodel.Duration(24 * time.Hour)})
			Expect(days.Id()).To(Equal(""))
			Expect(days.Expr()).To(Equal("sum(rate(hello_vector[1d]))"))

			_, err = days.Rule()
			Expect(err).To(HaveOccurred())
		})

		Specify("MWMB SLI generators should produce valid promQL", func() {
			mwmb := metrics.NewMWMBSLIGenerator(
				"sli_vector",
				"http_request_duration_seconds_count{job=\"myservice\", code=~\"5..|429\"}",
				"http_request_duration_seconds_count{job=\"myservice\"}",
				metrics.WindowMetadata{WindowDur: prommodel.Duration(time.Minute), Name: "page:quick:short"},
				map[string]string{
					"bar": "baz",
				},
			)

			Expect(mwmb.Id()).To(Equal("sli_vector:page:quick:short"))
			Expect(mwmb.Expr()).To(Equal(
				"1 - ((sum(rate(http_request_duration_seconds_count{job=\"myservice\", code=~\"5..|429\"}[1m]))) / (sum(rate(http_request_duration_seconds_count{job=\"myservice\"}[1m]))))",
			))

			_, err := promql.ParseExpr(mwmb.Expr())
			Expect(err).NotTo(HaveOccurred())

			rule, err := mwmb.Rule()
			Expect(err).NotTo(HaveOccurred())
			Expect(rule.Labels).To(Equal(map[string]string{
				"bar": "baz",
			}))

			_, err = promql.ParseExpr(rule.Record.Value)
			Expect(err).NotTo(HaveOccurred())

			seconds := metrics.NewMWMBSLIGenerator(
				"sli_vector",
				"http_request_duration_seconds_count{job=\"myservice\", code=~\"5..|429\"}",
				"http_request_duration_seconds_count{job=\"myservice\"}",
				metrics.WindowMetadata{WindowDur: prommodel.Duration(time.Second), Name: "ticket:long:long"},
				map[string]string{
					"bar": "baz",
				},
			)

			Expect(seconds.Id()).To(Equal("sli_vector:ticket:long:long"))
			Expect(seconds.Expr()).To(Equal(
				"1 - ((sum(rate(http_request_duration_seconds_count{job=\"myservice\", code=~\"5..|429\"}[1s]))) / (sum(rate(http_request_duration_seconds_count{job=\"myservice\"}[1s]))))",
			))

			_, err = promql.ParseExpr(seconds.Expr())
			Expect(err).NotTo(HaveOccurred())

			rule, err = seconds.Rule()
			Expect(err).NotTo(HaveOccurred())
			Expect(rule.Labels).To(Equal(map[string]string{
				"bar": "baz",
			}))

			_, err = promql.ParseExpr(rule.Record.Value)
			Expect(err).NotTo(HaveOccurred())

			hours := metrics.NewMWMBSLIGenerator(
				"sli_vector",
				"http_request_duration_seconds_count{job=\"myservice\", code=~\"5..|429\"}",
				"http_request_duration_seconds_count{job=\"myservice\"}",
				metrics.WindowMetadata{WindowDur: prommodel.Duration(time.Hour * 6), Name: "page:quick:short"},
				map[string]string{
					"bar": "baz",
				},
			)

			Expect(hours.Id()).To(Equal("sli_vector:page:quick:short"))
			Expect(hours.Expr()).To(Equal(
				"1 - ((sum(rate(http_request_duration_seconds_count{job=\"myservice\", code=~\"5..|429\"}[6h]))) / (sum(rate(http_request_duration_seconds_count{job=\"myservice\"}[6h]))))",
			))

			_, err = promql.ParseExpr(hours.Expr())
			Expect(err).NotTo(HaveOccurred())

			rule, err = hours.Rule()
			Expect(err).NotTo(HaveOccurred())
			Expect(rule.Labels).To(Equal(map[string]string{
				"bar": "baz",
			}))

			_, err = promql.ParseExpr(rule.Record.Value)
			Expect(err).NotTo(HaveOccurred())

			days := metrics.NewMWMBSLIGenerator(
				"sli_vector",
				"http_request_duration_seconds_count{job=\"myservice\", code=~\"5..|429\"}",
				"http_request_duration_seconds_count{job=\"myservice\"}",
				metrics.WindowMetadata{WindowDur: prommodel.Duration(time.Hour * 24 * 24), Name: "test"},
				map[string]string{
					"bar": "baz",
				},
			)

			Expect(days.Id()).To(Equal("sli_vector:test"))
			Expect(days.Expr()).To(Equal(
				"1 - ((sum(rate(http_request_duration_seconds_count{job=\"myservice\", code=~\"5..|429\"}[24d]))) / (sum(rate(http_request_duration_seconds_count{job=\"myservice\"}[24d]))))",
			))

			_, err = promql.ParseExpr(days.Expr())
			Expect(err).NotTo(HaveOccurred())

			rule, err = days.Rule()
			Expect(err).NotTo(HaveOccurred())
			Expect(rule.Labels).To(Equal(map[string]string{
				"bar": "baz",
			}))

			_, err = promql.ParseExpr(rule.Record.Value)
			Expect(err).NotTo(HaveOccurred())
		})

		Specify("Burn rate generators should generate valid promQL", func() {
			idLabels := map[string]string{
				"id": "1",
			}

			valueGenerator1 := metrics.NewConstantMetricGenerator(
				"test_metric",
				0.5,
				9,
				idLabels,
			)

			valueGenerator2 := metrics.NewConstantMetricGenerator(
				"test_metric2",
				0.4,
				9,
				idLabels,
			)

			opts := metrics.DefaultSLOGeneratorOptions()

			br := metrics.NewBurnRateGenerator(
				"burn_rate_current",
				idLabels,
				valueGenerator1,
				valueGenerator2,
				opts,
			)

			Expect(br.Id()).To(Equal("burn_rate_current"))
			Expect(br.Expr()).To(Equal("test_metric{id=~\"1\"} / on(id) group_left test_metric2{id=~\"1\"}"))

			_, err := promql.ParseExpr(br.Expr())
			Expect(err).NotTo(HaveOccurred())

			rule, err := br.Rule()
			Expect(err).NotTo(HaveOccurred())
			Expect(rule.Labels).To(Equal(idLabels))
			Expect(rule.Record.Value).To(Equal(br.Id()))
			Expect(rule.Expr.Value).To(Equal(br.Expr()))

			idLabels2 := map[string]string{
				"foo": "bar",
				"baz": "qux",
			}

			br2 := metrics.NewBurnRateGenerator(
				"burn_rate_current",
				idLabels2,
				valueGenerator1,
				valueGenerator2,
				opts,
			)

			Expect(br2.Id()).To(Equal("burn_rate_current"))
			Expect(br2.Expr()).To(Equal("test_metric{baz=~\"qux\",foo=~\"bar\"} / on(baz, foo) group_left test_metric2{baz=~\"qux\",foo=~\"bar\"}"))

			_, err = promql.ParseExpr(br2.Expr())
			Expect(err).NotTo(HaveOccurred())

			rule, err = br2.Rule()
			Expect(err).NotTo(HaveOccurred())
			Expect(rule.Labels).To(Equal(idLabels2))
			Expect(rule.Record.Value).To(Equal(br2.Id()))
			Expect(rule.Expr.Value).To(Equal(br2.Expr()))

			By("verifying unpotimized builds return runnable queries")

			opts.Apply(
				metrics.WithOptimization(false),
			)

			brUnopt := metrics.NewBurnRateGenerator(
				"burn_rate_current",
				idLabels2,
				valueGenerator1,
				valueGenerator2,
				opts,
			)
			Expect(brUnopt.Id()).To(Equal("burn_rate_current"))
			Expect(brUnopt.Expr()).To(Equal("(vector(0.500000000)) / (vector(0.400000000))"))

			_, err = promql.ParseExpr(brUnopt.Expr())
			Expect(err).NotTo(HaveOccurred())
		})

		Specify("Error budget generators should generate valid promQL", func() {
			opts := metrics.DefaultSLOGeneratorOptions()
			idLabels := map[string]string{
				"id":  "1",
				"foo": "bar",
			}
			periodQuery := metrics.NewConstantMetricGenerator(
				"test_metric",
				0.5,
				9,
				idLabels,
			)

			errB := metrics.NewErrorBudgetRemainingGenerator(
				"hello",
				idLabels,
				periodQuery,
				opts,
			)
			Expect(errB.Id()).To(Equal("hello"))
			Expect(errB.Expr()).To(Equal("1 - (test_metric{foo=~\"bar\",id=~\"1\"})"))

			_, err := promql.ParseExpr(errB.Expr())
			Expect(err).NotTo(HaveOccurred())

			rule, err := errB.Rule()
			Expect(err).NotTo(HaveOccurred())
			Expect(rule.Labels).To(Equal(idLabels))
			Expect(rule.Record.Value).To(Equal(errB.Id()))
			Expect(rule.Expr.Value).To(Equal(errB.Expr()))

			By("verifying unpotimized builds return runnable queries")

			opts.Apply(
				metrics.WithOptimization(false),
			)

			errBUnopt := metrics.NewErrorBudgetRemainingGenerator(
				"hello",
				idLabels,
				periodQuery,
				opts,
			)
			Expect(errBUnopt.Id()).To(Equal("hello"))
			Expect(errBUnopt.Expr()).To(Equal(fmt.Sprintf("1 - (%s)", periodQuery.Expr())))

			_, err = promql.ParseExpr(errBUnopt.Expr())
			Expect(err).NotTo(HaveOccurred())
		})

		Specify("MWMB alerts should generate valid promQL", func() {
			opts := metrics.DefaultSLOGeneratorOptions()
			idLabels := map[string]string{
				"id":  "1",
				"foo": "bar",
			}
			errorRate := func(window metrics.WindowMetadata) metrics.MetricGenerator {
				return metrics.NewMWMBSLIGenerator(
					"error_rate",
					"test_metric{foo=~\"bar\",id=~\"1\"}",
					"test_metric{foo=~\"bar\",id=~\"1|2\"}",
					window,
					idLabels,
				)
			}

			mwmb := metrics.WindowDefaults(prommodel.Duration(time.Hour * 24 * 30))
			quick, long := mwmb.PageWindows()
			pageGen := metrics.NewMWMBAlertGenerator(
				idLabels,
				quick,
				long,
				errorRate,
				opts,
				false,
			)

			Expect(pageGen.Id()).To(Equal("slo:mwmb_alert:page:quick"))
			GinkgoWriter.Write([]byte(pageGen.Expr()))
			expected := "(max(((error_rate:page:quick:short{foo=~\"bar\",id=~\"1\"}) > (172.800000000 * 2.000000000))) and max(((error_rate:page:quick:long{foo=~\"bar\",id=~\"1\"}) > (14.400000000 * 2.000000000)))) or (max((error_rate:page:slow:short{foo=~\"bar\",id=~\"1\"}) > (72.000000000 * 5.000000000)) and max((error_rate:page:slow:long{foo=~\"bar\",id=~\"1\"}) > (6.000000000 * 5.000000000)))"
			Expect(pageGen.Expr()).Should(Equal(expected))

			rule, err := pageGen.Rule()
			Expect(err).NotTo(HaveOccurred())

			_, err = promql.ParseExpr(rule.Expr.Value)
			Expect(err).NotTo(HaveOccurred())

			Expect(rule.Alert.Value).To(Equal(pageGen.Id()))
			Expect(rule.Expr.Value).To(Equal(pageGen.Expr()))
			Expect(rule.Labels).To(Equal(idLabels))

			By("verifying unpotimized MWMB alerts generate valid promQL")

			opts.Apply(
				metrics.WithOptimization(false),
			)
			pageGenRaw := metrics.NewMWMBAlertGenerator(
				idLabels,
				quick,
				long,
				errorRate,
				opts,
				false,
			)

			Expect(pageGenRaw.Id()).To(Equal("slo:mwmb_alert:page:quick"))
			expectedRaw := "(max(((1 - ((sum(rate(test_metric{foo=~\"bar\",id=~\"1\"}[5m]))) / (sum(rate(test_metric{foo=~\"bar\",id=~\"1|2\"}[5m]))))) > (172.800000000 * 2.000000000))) and max(((1 - ((sum(rate(test_metric{foo=~\"bar\",id=~\"1\"}[1h]))) / (sum(rate(test_metric{foo=~\"bar\",id=~\"1|2\"}[1h]))))) > (14.400000000 * 2.000000000)))) or (max((1 - ((sum(rate(test_metric{foo=~\"bar\",id=~\"1\"}[30m]))) / (sum(rate(test_metric{foo=~\"bar\",id=~\"1|2\"}[30m]))))) > (72.000000000 * 5.000000000)) and max((1 - ((sum(rate(test_metric{foo=~\"bar\",id=~\"1\"}[6h]))) / (sum(rate(test_metric{foo=~\"bar\",id=~\"1|2\"}[6h]))))) > (6.000000000 * 5.000000000)))"
			Expect(pageGenRaw.Expr()).Should(Equal(expectedRaw))

			rule, err = pageGenRaw.Rule()
			Expect(err).NotTo(HaveOccurred())

			_, err = promql.ParseExpr(rule.Expr.Value)
			Expect(err).NotTo(HaveOccurred())

			Expect(rule.Alert.Value).To(Equal(pageGenRaw.Id()))
			Expect(rule.Expr.Value).To(Equal(pageGenRaw.Expr()))
			Expect(rule.Labels).To(Equal(idLabels))

		})
	})

	When("we use the Prometheus SLO generator", func() {
		It("should generate valid MWMB optimized recording rules", func() {
			periods := []string{"7m", "7d", "30d"}
			for _, period := range periods {
				sloUuid := uuid.New().String()
				idLabels := backend.IdentificationLabels(map[string]string{
					backend.SLOUuid:    sloUuid,
					backend.SLOName:    "test",
					backend.SLOService: "scrape",
				})
				curSloFilters := idLabels.ToLabels().ConstructPrometheus()
				Expect(curSloFilters).To(Equal(fmt.Sprintf("slo_opni_id=~\"%s\",slo_opni_name=~\"test\",slo_opni_service=~\"scrape\"", sloUuid)))
				curJoinFilters := idLabels.JoinOnPrometheus()
				Expect(curJoinFilters).To(Equal("slo_opni_id, slo_opni_name, slo_opni_service"))

				sloGen, err := metrics.NewSLOGenerator(
					backend.SLO{
						SloPeriod:   period,
						Objective:   99.9,
						Svc:         "scrape",
						GoodMetric:  "test_metric",
						TotalMetric: "test_metric",
						IdLabels:    idLabels,
						UserLabels:  map[string]string{},
						GoodEvents: []backend.LabelPair{
							{
								Key:  "foo",
								Vals: []string{"bar"},
							},
						},
						TotalEvents: []backend.LabelPair{},
					},
				)
				Expect(err).NotTo(HaveOccurred())
				windows := sloGen.Windows()
				Expect(windows).NotTo(HaveLen(0))
				Expect(len(windows)).To(BeNumerically(">", 2))

				for _, window := range windows {

					gen := sloGen.SLI(window)
					Expect(gen).NotTo(BeNil())
					Expect(gen.Id()).NotTo(Equal(""))
					Expect(gen.Id()).To(HavePrefix(metrics.SLOSLI))
					Expect(gen.Id()).To(HaveSuffix(window.Name))
					Expect(gen.Id()).To(Equal(metrics.SLOSLI + ":" + window.Name))
					expected := fmt.Sprintf("1 - ((sum(rate(test_metric{job=\"scrape\", foo=~\"bar\"}[%s]))) / (sum(rate(test_metric{job=\"scrape\"}[%s]))))", prommodel.Duration(window.WindowDur), prommodel.Duration(window.WindowDur))
					Expect(gen.Expr()).To(Equal(expected))
					_, err = promql.ParseExpr(gen.Expr())
					Expect(err).NotTo(HaveOccurred())
					rule, err := gen.Rule()
					Expect(err).NotTo(HaveOccurred())
					Expect(rule.Record.Value).To(Equal(gen.Id()))
					Expect(rule.Expr.Value).To(Equal(gen.Expr()))
					Expect(rule.Labels).To(Equal(
						map[string]string{
							backend.SLOUuid:    sloUuid,
							backend.SLOName:    "test",
							backend.SLOService: "scrape",
						},
					))
				}

				By("expecting the constant value rules are correct")
				infoVec := sloGen.Info()
				Expect(infoVec.Id()).To(Equal(metrics.SLOInfo))
				Expect(infoVec.Expr()).To(Equal("vector(1.0)"))

				_, err = promql.ParseExpr(infoVec.Expr())
				Expect(err).NotTo(HaveOccurred())

				rule, err := infoVec.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(infoVec.Id()))
				Expect(rule.Expr.Value).To(Equal(infoVec.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))

				periodVec := sloGen.Period()
				periodFloat := time.Duration(util.Must(prommodel.ParseDuration(period))).Seconds()
				Expect(periodVec.Id()).To(Equal(metrics.SLOPeriod))
				Expect(periodVec.Expr()).To(Equal(fmt.Sprintf("vector(%.9f)", periodFloat)))

				_, err = promql.ParseExpr(periodVec.Expr())
				Expect(err).NotTo(HaveOccurred())
				rule, err = periodVec.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(periodVec.Id()))
				Expect(rule.Expr.Value).To(Equal(periodVec.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))

				objective := sloGen.Objective()
				Expect(objective.Id()).To(Equal(metrics.SLOObjective))
				Expect(objective.Expr()).To(Equal("vector(0.999000000)"))

				_, err = promql.ParseExpr(objective.Expr())
				Expect(err).NotTo(HaveOccurred())
				rule, err = objective.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(objective.Id()))

				errorBudget := sloGen.ErrorBudget()
				Expect(errorBudget.Id()).To(Equal(metrics.SLOErrorBudget))
				Expect(errorBudget.Expr()).To(Equal("vector(0.001000000)"))

				_, err = promql.ParseExpr(errorBudget.Expr())
				Expect(err).NotTo(HaveOccurred())

				rule, err = errorBudget.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(errorBudget.Id()))
				Expect(rule.Expr.Value).To(Equal(errorBudget.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))

				errorBudgetId := sloGen.ErrorBudget().Id()
				By("expecting the optmimized current burn rate rule to reference the correct sli rule")
				cbr := sloGen.CurrentBurnRate()
				Expect(cbr.Id()).To(Equal(metrics.SLOCurrentBurnRate))
				quickestSLIId := sloGen.SLI(sloGen.Windows()[0]).Id()
				expected := fmt.Sprintf(
					`%s{%s} / on(%s) group_left %s{%s}`,
					quickestSLIId,
					curSloFilters,
					curJoinFilters,
					errorBudgetId,
					curSloFilters,
				)
				Expect(cbr.Expr()).To(Equal(expected))

				_, err = promql.ParseExpr(cbr.Expr())
				Expect(err).NotTo(HaveOccurred())

				rule, err = cbr.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(cbr.Id()))
				Expect(rule.Expr.Value).To(Equal(cbr.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))

				By("expecting the optimized period burn rate rule to reference the correct sli rule")
				pbr := sloGen.PeriodBurnRate()

				periodSLIID := sloGen.SLI(metrics.WindowMetadata{WindowDur: util.Must(prommodel.ParseDuration(period)), Name: "period"}).Id()

				expectedP := fmt.Sprintf(
					`%s{%s} / on(%s) group_left %s{%s}`,
					periodSLIID,
					curSloFilters,
					curJoinFilters,
					errorBudgetId,
					curSloFilters,
				)
				GinkgoWriter.Write([]byte(expectedP))
				Expect(pbr.Expr()).To(Equal(expectedP))

				_, err = promql.ParseExpr(pbr.Expr())
				Expect(err).NotTo(HaveOccurred())

				rule, err = pbr.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(pbr.Id()))
				Expect(rule.Expr.Value).To(Equal(pbr.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))

				By("expecting the optimized error budget remaining rules references the correct metrics")

				errBr := sloGen.ErrorBudgetRemaining()
				Expect(errBr.Id()).To(Equal(metrics.SLOErrorBudgetRemaining))
				Expect(errBr.Expr()).To(Equal(fmt.Sprintf("1 - (%s{%s})", pbr.Id(), curSloFilters)))
				_, err = promql.ParseExpr(errBr.Expr())
				Expect(err).NotTo(HaveOccurred())

				rule, err = errBr.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(errBr.Id()))
				Expect(rule.Expr.Value).To(Equal(errBr.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))
			}
		})

		It("should generate valid unoptimized MWMB recording rules", func() {
			dur := prommodel.Duration((time.Minute*14 + (time.Second * 24)) * 10)
			periods := []string{dur.String(), "7d", "30d"}
			for _, period := range periods {
				sloUuid := uuid.New().String()
				idLabels := backend.IdentificationLabels(map[string]string{
					backend.SLOUuid:    sloUuid,
					backend.SLOName:    "test",
					backend.SLOService: "scrape",
				})
				curSloFilters := idLabels.ToLabels().ConstructPrometheus()
				Expect(curSloFilters).To(Equal(fmt.Sprintf("slo_opni_id=~\"%s\",slo_opni_name=~\"test\",slo_opni_service=~\"scrape\"", sloUuid)))
				curJoinFilters := idLabels.JoinOnPrometheus()
				Expect(curJoinFilters).To(Equal("slo_opni_id, slo_opni_name, slo_opni_service"))

				sloGen, err := metrics.NewSLOGenerator(
					backend.SLO{
						SloPeriod:   period,
						Objective:   99.9,
						Svc:         "scrape",
						GoodMetric:  "test_metric",
						TotalMetric: "test_metric",
						IdLabels:    idLabels,
						UserLabels:  map[string]string{},
						GoodEvents: []backend.LabelPair{
							{
								Key:  "foo",
								Vals: []string{"bar"},
							},
							{
								Key:  "code",
								Vals: []string{"200"},
							},
						},
						TotalEvents: []backend.LabelPair{
							{
								Key:  "code",
								Vals: []string{"200", "500", "503"},
							},
						},
					},
					metrics.WithOptimization(false),
				)
				Expect(err).NotTo(HaveOccurred())
				windows := sloGen.Windows()
				Expect(windows).NotTo(HaveLen(0))
				Expect(len(windows)).To(BeNumerically(">", 2))

				for _, window := range windows {
					gen := sloGen.SLI(window)
					Expect(gen).NotTo(BeNil())
					Expect(gen.Id()).NotTo(Equal(""))
					Expect(gen.Id()).To(HavePrefix(metrics.SLOSLI))
					Expect(gen.Id()).To(HaveSuffix(window.Name))
					Expect(gen.Id()).To(Equal(metrics.SLOSLI + ":" + window.Name))
					expected := fmt.Sprintf("1 - ((sum(rate(test_metric{job=\"scrape\", foo=~\"bar\",code=~\"200\"}[%s]))) / (sum(rate(test_metric{job=\"scrape\", code=~\"200|500|503\"}[%s]))))", prommodel.Duration(window.WindowDur), prommodel.Duration(window.WindowDur))
					Expect(gen.Expr()).To(Equal(expected))
					_, err := promql.ParseExpr(gen.Expr())
					Expect(err).NotTo(HaveOccurred())
					rule, err := gen.Rule()
					Expect(err).NotTo(HaveOccurred())
					Expect(rule.Record.Value).To(Equal(gen.Id()))
					Expect(rule.Expr.Value).To(Equal(gen.Expr()))
					Expect(rule.Labels).To(Equal(
						map[string]string{
							backend.SLOUuid:    sloUuid,
							backend.SLOName:    "test",
							backend.SLOService: "scrape",
						},
					))
				}

				By("expecting the constant value rules are correct")
				infoVec := sloGen.Info()
				Expect(infoVec.Id()).To(Equal(metrics.SLOInfo))
				Expect(infoVec.Expr()).To(Equal("vector(1.0)"))

				_, err = promql.ParseExpr(infoVec.Expr())
				Expect(err).NotTo(HaveOccurred())

				rule, err := infoVec.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(infoVec.Id()))
				Expect(rule.Expr.Value).To(Equal(infoVec.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))

				periodVec := sloGen.Period()
				periodFloat := time.Duration(util.Must(prommodel.ParseDuration(period))).Seconds()
				Expect(periodVec.Id()).To(Equal(metrics.SLOPeriod))
				Expect(periodVec.Expr()).To(Equal(fmt.Sprintf("vector(%.9f)", periodFloat)))

				_, err = promql.ParseExpr(periodVec.Expr())
				Expect(err).NotTo(HaveOccurred())
				rule, err = periodVec.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(periodVec.Id()))
				Expect(rule.Expr.Value).To(Equal(periodVec.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))

				objective := sloGen.Objective()
				Expect(objective.Id()).To(Equal(metrics.SLOObjective))
				Expect(objective.Expr()).To(Equal("vector(0.999000000)"))

				_, err = promql.ParseExpr(objective.Expr())
				Expect(err).NotTo(HaveOccurred())
				rule, err = objective.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(objective.Id()))

				errorBudget := sloGen.ErrorBudget()
				Expect(errorBudget.Id()).To(Equal(metrics.SLOErrorBudget))
				Expect(errorBudget.Expr()).To(Equal("vector(0.001000000)"))

				_, err = promql.ParseExpr(errorBudget.Expr())
				Expect(err).NotTo(HaveOccurred())

				rule, err = errorBudget.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(errorBudget.Id()))
				Expect(rule.Expr.Value).To(Equal(errorBudget.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))

				errorBudgetExpr := sloGen.ErrorBudget().Expr()
				By("expecting the unoptmimized current burn rate rule to be valid promQL")
				cbr := sloGen.CurrentBurnRate()
				Expect(cbr.Id()).To(Equal(metrics.SLOCurrentBurnRate))
				quickestSLIExpr := sloGen.SLI(sloGen.Windows()[0]).Expr()
				expected := fmt.Sprintf("(%s) / (%s)", quickestSLIExpr, errorBudgetExpr)
				Expect(cbr.Expr()).To(Equal(expected))

				_, err = promql.ParseExpr(cbr.Expr())
				Expect(err).NotTo(HaveOccurred())

				rule, err = cbr.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(cbr.Id()))
				Expect(rule.Expr.Value).To(Equal(cbr.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))

				By("expecting the unoptimized period burn rate rule to be valid promql")
				pbr := sloGen.PeriodBurnRate()
				periodSLIExpr := sloGen.SLI(metrics.WindowMetadata{WindowDur: util.Must(prommodel.ParseDuration(period)), Name: "test"}).Expr()

				expectedP := fmt.Sprintf(
					`(%s) / (%s)`,
					periodSLIExpr,
					errorBudgetExpr,
				)
				GinkgoWriter.Write([]byte(expectedP))
				Expect(pbr.Expr()).To(Equal(expectedP))

				_, err = promql.ParseExpr(pbr.Expr())
				Expect(err).NotTo(HaveOccurred())

				rule, err = pbr.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(pbr.Id()))
				Expect(rule.Expr.Value).To(Equal(pbr.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))

				By("expecting the unoptimized error budget remaining rules references the correct metrics")

				errBr := sloGen.ErrorBudgetRemaining()
				Expect(errBr.Id()).To(Equal(metrics.SLOErrorBudgetRemaining))
				Expect(errBr.Expr()).To(Equal(fmt.Sprintf("1 - (%s)", pbr.Expr())))

				_, err = promql.ParseExpr(errBr.Expr())
				Expect(err).NotTo(HaveOccurred())

				rule, err = errBr.Rule()
				Expect(err).NotTo(HaveOccurred())
				Expect(rule.Record.Value).To(Equal(errBr.Id()))
				Expect(rule.Expr.Value).To(Equal(errBr.Expr()))
				Expect(rule.Labels).To(Equal(
					map[string]string{
						backend.SLOUuid:    sloUuid,
						backend.SLOName:    "test",
						backend.SLOService: "scrape",
					},
				))
			}
		})
		It("should generate valid optimized rule groups", func() {
			sloUuid := uuid.New().String()
			idLabels := map[string]string{
				backend.SLOUuid:    sloUuid,
				backend.SLOName:    "test",
				backend.SLOService: "scrape",
			}
			curSloFilters := backend.IdentificationLabels(idLabels).ToLabels().ConstructPrometheus()
			Expect(curSloFilters).To(Equal(fmt.Sprintf("slo_opni_id=~\"%s\",slo_opni_name=~\"test\",slo_opni_service=~\"scrape\"", sloUuid)))
			curJoinFilters := backend.IdentificationLabels(idLabels).JoinOnPrometheus()
			Expect(curJoinFilters).To(Equal("slo_opni_id, slo_opni_name, slo_opni_service"))

			period := prommodel.Duration((time.Minute*14 + (time.Second * 24)) * 10)

			sloGen, err := metrics.NewSLOGenerator(
				backend.SLO{
					SloPeriod:   period.String(),
					Objective:   99.9,
					Svc:         "scrape",
					GoodMetric:  "test_metric",
					TotalMetric: "test_metric",
					IdLabels:    idLabels,
					UserLabels:  map[string]string{},
					GoodEvents: []backend.LabelPair{
						{
							Key:  "foo",
							Vals: []string{"bar"},
						},
						{
							Key:  "code",
							Vals: []string{"200"},
						},
					},
					TotalEvents: []backend.LabelPair{
						{
							Key:  "code",
							Vals: []string{"200", "500", "503"},
						},
					},
				},
				metrics.WithOptimization(false),
			)
			Expect(err).NotTo(HaveOccurred())

			rules, err := sloGen.AsRuleGroup()
			Expect(err).NotTo(HaveOccurred())
			Expect(rules).NotTo(BeNil())
			Expect(rules.Name).To(ContainSubstring(sloUuid))
			Expect(rules.Rules).NotTo(HaveLen(0))

			By("verifying the ouput rule groups contain the correct rules")
			ruleIds := lo.Map(rules.Rules, func(r rulefmt.RuleNode, _ int) string {
				if r.Record.Value == "" {
					return r.Alert.Value
				}
				return r.Record.Value
			})

			generatorIds := []string{
				sloGen.Info().Id(),
				sloGen.Objective().Id(),
				sloGen.ErrorBudget().Id(),
				sloGen.Period().Id(),
				sloGen.CurrentBurnRate().Id(),
				sloGen.PeriodBurnRate().Id(),
				sloGen.ErrorBudgetRemaining().Id(),
			}
			for _, window := range append(sloGen.Windows()) {
				generatorIds = append(generatorIds, sloGen.SLI(window).Id())
			}

			Expect(ruleIds).To(ConsistOf(generatorIds))

			By("verifying the contents are valid promQL and are labelled with the correct ID")

			for _, rule := range rules.Rules {
				_, err := promql.ParseExpr(rule.Expr.Value)
				Expect(err).NotTo(HaveOccurred())

				Expect(rule.Labels).To(Equal(
					idLabels,
				))
			}
		})

		It("should generated valid unoptimized rule groups", func() {
			sloUuid := uuid.New().String()
			idLabels := map[string]string{
				backend.SLOUuid:    sloUuid,
				backend.SLOName:    "test",
				backend.SLOService: "scrape",
			}
			curSloFilters := backend.IdentificationLabels(idLabels).ToLabels().ConstructPrometheus()
			Expect(curSloFilters).To(Equal(fmt.Sprintf("slo_opni_id=~\"%s\",slo_opni_name=~\"test\",slo_opni_service=~\"scrape\"", sloUuid)))
			curJoinFilters := backend.IdentificationLabels(idLabels).JoinOnPrometheus()
			Expect(curJoinFilters).To(Equal("slo_opni_id, slo_opni_name, slo_opni_service"))

			period := prommodel.Duration((time.Minute*14 + (time.Second * 24)) * 10)

			sloGen, err := metrics.NewSLOGenerator(
				backend.SLO{
					SloPeriod:   period.String(),
					Objective:   99.9,
					Svc:         "scrape",
					GoodMetric:  "test_metric",
					TotalMetric: "test_metric",
					IdLabels:    idLabels,
					UserLabels:  map[string]string{},
					GoodEvents: []backend.LabelPair{
						{
							Key:  "foo",
							Vals: []string{"bar"},
						},
						{
							Key:  "code",
							Vals: []string{"200"},
						},
					},
					TotalEvents: []backend.LabelPair{
						{
							Key:  "code",
							Vals: []string{"200", "500", "503"},
						},
					},
				},
				metrics.WithOptimization(false),
			)
			Expect(err).NotTo(HaveOccurred())

			rules, err := sloGen.AsRuleGroup()
			Expect(err).NotTo(HaveOccurred())
			Expect(rules).NotTo(BeNil())
			Expect(rules.Name).To(ContainSubstring(sloUuid))
			Expect(rules.Rules).NotTo(HaveLen(0))

			By("verifying the ouput rule groups contain the correct rules")
			ruleIds := lo.Map(rules.Rules, func(r rulefmt.RuleNode, _ int) string {
				if r.Record.Value == "" {
					return r.Alert.Value
				}
				return r.Record.Value
			})

			generatorIds := []string{
				sloGen.Info().Id(),
				sloGen.Objective().Id(),
				sloGen.ErrorBudget().Id(),
				sloGen.Period().Id(),
				sloGen.CurrentBurnRate().Id(),
				sloGen.PeriodBurnRate().Id(),
				sloGen.ErrorBudgetRemaining().Id(),
			}
			for _, window := range append(sloGen.Windows()) {
				generatorIds = append(generatorIds, sloGen.SLI(window).Id())
			}

			Expect(ruleIds).To(ConsistOf(generatorIds))

			By("verifying the contents are valid promQL and are labelled with the correct ID")

			for _, rule := range rules.Rules {
				_, err := promql.ParseExpr(rule.Expr.Value)
				Expect(err).NotTo(HaveOccurred())

				Expect(rule.Labels).To(Equal(
					idLabels,
				))
			}
		})
	})
})
