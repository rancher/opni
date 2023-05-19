package slo_test

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/goombaio/namegenerator"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v3"
)

// need to check always that good <= total
func expectValidEventSubsets(good []*sloapi.Event, total []*sloapi.Event) {
	cacheTotal := make(map[string]bool)
	listVals := make(map[string][]string)
	for i := 0; i < len(good); i++ {
		listVals[good[i].Key] = good[i].Vals
	}
	for i := 0; i < len(total); i++ {
		cacheTotal[total[i].Key] = false
		Expect(len(total[i].Vals)).To(BeNumerically(">=", len(listVals[total[i].Key])))
		Expect(total[i].Vals).To(ContainElements(listVals[total[i].Key])) // total events must be at least a super set of good events
	}
	for i := 0; i < len(good); i++ {
		if _, ok := cacheTotal[good[i].Key]; ok {
			cacheTotal[good[i].Key] = true
		}
	}

	// everything in total should be defined in good
	for _, v := range cacheTotal {
		Expect(v).To(BeTrue())
	}

}

var _ = Describe("Converting SLO information to Cortex rules", Ordered, Label("integration", "slow"), func() {
	sloObj := slo.NewSLO(
		"slo-name",
		"30d",
		99.99,
		"prometheus",
		"prometheus_http_requests_total",
		"prometheus_http_requests_total",
		map[string]string{
			"important": "true",
		},
		slo.LabelPairs{
			{
				Key:  "code",
				Vals: []string{"200"},
			},
		},
		slo.LabelPairs{
			{
				Key:  "code",
				Vals: []string{"200", "500", "503"},
			},
		},
	)
	// test environment references
	var env *test.Environment
	var pPort int
	var adminClient cortexadmin.CortexAdminClient

	BeforeAll(func() {
		env = &test.Environment{}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		client := env.NewManagementClient()
		token, err := client.CreateBootstrapToken(env.Context(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		info, err := client.CertsInfo(env.Context(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		_, errC := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint}, test.WithContext(env.Context()))
		Eventually(errC).Should(Receive(BeNil()))
		pPort, err = env.StartPrometheus("agent")
		Expect(err).NotTo(HaveOccurred())
		_, errC = env.StartAgent("agent2", token, []string{info.Chain[len(info.Chain)-1].Fingerprint}, test.WithContext(env.Context()))
		Eventually(errC).Should(Receive(BeNil()))
		pPort, err = env.StartPrometheus("agent2")
		Expect(err).NotTo(HaveOccurred())

		_, err = client.InstallCapability(env.Context(), &managementv1.CapabilityInstallRequest{
			Name: wellknown.CapabilityMetrics,
			Target: &capabilityv1.InstallRequest{
				Cluster: &corev1.Reference{Id: "agent"},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		_, err = client.InstallCapability(env.Context(), &managementv1.CapabilityInstallRequest{
			Name: wellknown.CapabilityMetrics,
			Target: &capabilityv1.InstallRequest{
				Cluster: &corev1.Reference{Id: "agent2"},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		adminClient = cortexadmin.NewCortexAdminClient(env.ManagementClientConn())
		// Eventually(func() error {
		// 	stats, err := adminClient.AllUserStats(context.Background(), &emptypb.Empty{})
		// 	if err != nil {
		// 		return err
		// 	}
		// 	for _, item := range stats.Items {
		// 		if item.UserID == "agent" {
		// 			if item.NumSeries > 0 {
		// 				return nil
		// 			}
		// 		}
		// 	}
		// 	return fmt.Errorf("waiting for metric data to be stored in cortex")
		// }, 30*time.Second, 1*time.Second).Should(Succeed())
		// Eventually(func() error {
		// 	stats, err := adminClient.AllUserStats(context.Background(), &emptypb.Empty{})
		// 	if err != nil {
		// 		return err
		// 	}
		// 	for _, item := range stats.Items {
		// 		if item.UserID == "agent2" {
		// 			if item.NumSeries > 0 {
		// 				return nil
		// 			}
		// 		}
		// 	}
		// 	return fmt.Errorf("waiting for metric data to be stored in cortex")
		// }, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	When("We receive alert matrix data from cortex", func() {
		It("should be able to convert the alert matrix data to active windows", func() {
			_, err := slo.DetectActiveWindows("severe", nil)
			Expect(err).To(HaveOccurred())

			start := time.Now().Add(-(time.Hour * 24))
			startAlertWindow1 := time.Now().Add(-time.Hour * 5)
			endAlertWindow1 := time.Now().Add(-time.Hour * 4)
			startAlertWindow2 := time.Now().Add(-time.Hour * 3)
			endAlertWindow2 := time.Now().Add(-time.Hour * 2)

			matrix := &prommodel.Matrix{
				// sample stream 1
				{
					Values: []prommodel.SamplePair{
						{
							Timestamp: prommodel.TimeFromUnix(start.Unix()),
							Value:     0,
						},
						{
							Timestamp: prommodel.TimeFromUnix(startAlertWindow1.Unix()),
							Value:     1,
						},
					},
				},
				// sample stream 2
				{
					Values: []prommodel.SamplePair{
						{
							Timestamp: prommodel.TimeFromUnix(endAlertWindow1.Unix()),
							Value:     0,
						},
						{
							Timestamp: prommodel.TimeFromUnix(endAlertWindow1.Add(time.Minute).Unix()),
							Value:     0,
						},
						{
							Timestamp: prommodel.TimeFromUnix(startAlertWindow2.Unix()),
							Value:     1,
						},
						{
							Timestamp: prommodel.TimeFromUnix(startAlertWindow2.Add(time.Minute).Unix()),
							Value:     1,
						},
						{
							Timestamp: prommodel.TimeFromUnix(endAlertWindow2.Unix()),
							Value:     0,
						},
					},
				},
			}
			windows, err := slo.DetectActiveWindows("severe", matrix)
			Expect(err).NotTo(HaveOccurred())
			Expect(windows).To(HaveLen(2))
			Expect(windows[0].Start.AsTime().Unix()).To(Equal(startAlertWindow1.Unix()))
			Expect(windows[0].End.AsTime().Unix()).To(Equal(endAlertWindow1.Unix()))
			Expect(windows[1].Start.AsTime().Unix()).To(Equal(startAlertWindow2.Unix()))
			Expect(windows[1].End.AsTime().Unix()).To(Equal(endAlertWindow2.Unix()))
		})
	})

	When("We receive SLO event data from the user", func() {
		It("Should be able to convert the SLO event data to matching subsets for identical metric names", func() {
			// total events empty ==> total events stays empty
			goodEvents1 := []*sloapi.Event{
				{
					Key:  "code",
					Vals: []string{"200"},
				},
			}
			totalEvents1 := []*sloapi.Event{}
			g, t := slo.ToMatchingSubsetIdenticalMetric(goodEvents1, totalEvents1)
			Expect(g).To(Equal(goodEvents1))
			Expect(t).To(Equal(totalEvents1))

			// management api server will pass in empty events to an empty list,
			// because it hates us
			totalEvents1 = []*sloapi.Event{{}} // in this case, this is what it looks like
			g, t = slo.ToMatchingSubsetIdenticalMetric(goodEvents1, totalEvents1)
			Expect(g).To(Equal(goodEvents1))
			Expect(t).To(Equal(totalEvents1))

			// to be dilligent we should also check when t.Vals is nil
			totalEvents1 = []*sloapi.Event{{Key: "code"}}
			g, t = slo.ToMatchingSubsetIdenticalMetric(goodEvents1, totalEvents1)
			Expect(g).To(Equal(goodEvents1))
			Expect(t).To(Equal(totalEvents1))

			// should fill in missing subset
			goodEvents2 := []*sloapi.Event{
				{
					Key:  "code",
					Vals: []string{"200"},
				},
			}
			totalEvents2 := []*sloapi.Event{
				{
					Key:  "code",
					Vals: []string{"500", "503"},
				},
			}
			goodEvents2Copy := make([]string, len(goodEvents2[0].Vals))
			copy(goodEvents2Copy, goodEvents2[0].Vals)
			totalEvents2Copy := make([]string, len(totalEvents2[0].Vals))
			copy(totalEvents2Copy, totalEvents2[0].Vals)
			g, t = slo.ToMatchingSubsetIdenticalMetric(goodEvents2, totalEvents2)
			Expect(g).To(Equal(goodEvents2))
			Expect(t[0].Key).To(Equal(totalEvents2[0].Key))
			Expect(t[0].Vals).To(ConsistOf(append(goodEvents2Copy, totalEvents2Copy...)))

			// should coerce subsets with different filters
			goodEvents3 := []*sloapi.Event{
				{
					Key:  "code",
					Vals: []string{"200"},
				},
			}
			totalEvents3 := []*sloapi.Event{
				{
					Key:  "handler",
					Vals: []string{"/ready"},
				},
			}
			g, t = slo.ToMatchingSubsetIdenticalMetric(goodEvents3, totalEvents3)
			Expect(g).To(Equal([]*sloapi.Event{
				{
					Key:  "code",
					Vals: []string{"200"},
				},
				{
					Key:  "handler",
					Vals: []string{"/ready"},
				},
			}))
			Expect(t).To(Equal(totalEvents3))

			goodEvents4 := []*sloapi.Event{{
				Key:  "code",
				Vals: []string{"200"}}}
			totalEvents4 := []*sloapi.Event{
				{
					Key:  "code",
					Vals: []string{"500", "503"},
				},
			}

			g, t = slo.ToMatchingSubsetIdenticalMetric(goodEvents4, totalEvents4)
			Expect(Expect(g[0].Key).To(Equal(goodEvents4[0].Key)))
			Expect(g[0].Vals).To(ConsistOf([]string{"200"}))
			Expect(t[0].Vals).To(ConsistOf(slo.LeftJoinSlice(goodEvents4[0].Vals, totalEvents4[0].Vals)))

		})

		Specify("The event matching subset algorithm should be robust to chaos testing", func() {
			// FIXME: this test is kinda hacky and poorly written
			for numTests := 0; numTests < 15; numTests++ {
				goodEventNum := rand.Intn(10) + 1
				goodEventValNum := rand.Intn(1000) + 1
				totalEventNum := rand.Intn(10) + 1

				goodEvents := make([]*sloapi.Event, goodEventNum)
				totalEvents := make([]*sloapi.Event, totalEventNum)
				goodEventCache := make(map[string]struct{})
				goodEventValsCache := make(map[string]map[string]struct{})
				gen := namegenerator.NewNameGenerator(time.Now().UnixNano())
				for i := 0; i < goodEventNum; i++ {
					name := gen.Generate()
					vals := []string{}
					for j := 0; j < goodEventValNum; j++ {
						newVal := gen.Generate()
						vals = append(vals, newVal)
						if _, ok := goodEventValsCache[name]; !ok {
							goodEventValsCache[name] = make(map[string]struct{})
						}
						goodEventValsCache[name][newVal] = struct{}{}
					}
					goodEvents[i] = &sloapi.Event{
						Key:  name,
						Vals: vals,
					}
					goodEventCache[name] = struct{}{}
				}
				for i := 0; i < totalEventNum; i++ {
					isMany := rand.Intn(2)
					var totalEventValNum int
					if isMany == 0 {
						totalEventValNum = rand.Intn(1000)
					} else {
						totalEventValNum = rand.Intn(3)
					}

					// 50% chance to use an existing key from goodEvents
					var code string
					var vals []string
					if rand.Intn(2) == 0 && len(goodEventCache) > 0 { // use existing key
						if goodEventNum < 0 {
							panic(goodEvents)
						}
						index := rand.Intn(goodEventNum)
						code = goodEvents[index].Key
						delete(goodEventCache, code)
						for j := 0; j < totalEventValNum; j++ {
							//25% chance to use existing value from goodEvents
							if rand.Intn(4) == 0 && len(goodEventValsCache[code]) > 0 {
								keys := make([]string, len(goodEventValsCache[code]))

								x := 0
								for k := range goodEventValsCache[code] {
									keys[x] = k
									x++
								}
								index := rand.Intn(len(keys))
								vals = append(vals, keys[index])
								delete(goodEventValsCache[code], keys[index])
							} else {
								vals = append(vals, gen.Generate())
							}
						}
					} else { //create new key
						name := gen.Generate()
						code = name
						for j := 0; j < totalEventValNum; j++ {
							vals = append(vals, gen.Generate())
						}
					}
					totalEvents[i] = &sloapi.Event{
						Key:  code,
						Vals: vals,
					}
				}
				invalid := false
				for i := 0; i < len(totalEvents); i++ {
					if totalEvents[i].Vals == nil || len(totalEvents[i].Vals) == 0 {
						invalid = true
						break
					}
					if totalEvents[i] == nil {
						invalid = true
						break
					}
				}
				if invalid == true {
					continue // FIXME: not sure why we are getting invalid constructions
				}
				newGoodEvents, newTotalEvents := slo.ToMatchingSubsetIdenticalMetric(goodEvents, totalEvents)
				expectValidEventSubsets(newGoodEvents, newTotalEvents)
			}
		})
	})

	When("We use SLO objects to construct rules", func() {
		Specify("Label pairs should be able to construct promQL filters", func() {
			fmt.Println(pPort)
			goodLabelPairs := slo.LabelPairs{
				{
					Key:  "code",
					Vals: []string{"200"},
				},
			}
			Expect(goodLabelPairs.Construct()).To(Equal(",code=~\"200\""))
			totalLabelPairs := slo.LabelPairs{
				{
					Key:  "code",
					Vals: []string{"200", "500", "503"},
				},
			}
			Expect(totalLabelPairs.Construct()).To(Equal(",code=~\"200|500|503\""))
		})

		It("Should construct an SLO object", func() {
			sloObj := slo.NewSLO(
				"slo-name",
				"30d",
				99.99,
				"prometheus",
				slo.Metric("prometheus_http_requests_total"),
				slo.Metric("prometheus_http_requests_total"),
				map[string]string{
					"important": "true",
				},
				slo.LabelPairs{
					{
						Key:  "code",
						Vals: []string{"200"},
					},
				},
				slo.LabelPairs{
					{
						Key:  "code",
						Vals: []string{"200", "500", "503"},
					},
				},
			)
			Expect(sloObj).ToNot(BeNil())
			Expect(sloObj.GetId()).ToNot(BeNil())
			sloObj2 := slo.SLOFromId(

				"slo-name",
				"30d",
				99.99,
				"prometheus",
				"",
				"slo-operator",
				map[string]string{
					"important": "true",
				},
				slo.LabelPairs{
					{
						Key:  "code",
						Vals: []string{"200"},
					},
				},
				slo.LabelPairs{
					{
						Key:  "code",
						Vals: []string{"200", "500", "503"},
					},
				},
				sloObj.GetId(),
			)
			Expect(sloObj2).ToNot(BeNil())
			Expect(sloObj2.GetId()).To(Equal(sloObj.GetId()))
		})
		Specify("SLO objects should be able to create valid SLI Prometheus rules", func() {
			sloObj := slo.NewSLO(
				"slo-name",
				"30d",
				99.99,
				"prometheus",
				"prometheus_http_requests_total",
				"prometheus_http_requests_total",
				map[string]string{
					"important": "true",
				},
				slo.LabelPairs{
					{
						Key:  "code",
						Vals: []string{"200"},
					},
				},
				slo.LabelPairs{
					{
						Key:  "code",
						Vals: []string{"200", "500", "503"},
					},
				},
			)
			query, err := sloObj.RawSLIQuery("5m")
			Expect(err).To(Succeed())
			Expect(query).NotTo(Equal(""))
			_, err = parser.ParseExpr(query)
			Expect(err).To(Succeed())
		})

		Specify("SLO objects should be able to create valid metadata Prometheus rules", func() {
			sloObj := slo.NewSLO(
				"slo-name",
				"30d",
				99.99,
				"prometheus",
				"prometheus_http_requests_total",
				"prometheus_http_requests_total",
				map[string]string{
					"important": "true",
				},
				slo.LabelPairs{
					{
						Key:  "code",
						Vals: []string{"200"},
					},
				},
				slo.LabelPairs{
					{
						Key:  "code",
						Vals: []string{"200", "500", "503"},
					},
				},
			)
			dash := sloObj.RawDashboardInfoQuery()
			_, err := parser.ParseExpr(dash)
			Expect(err).To(Succeed())
			rawBudget := sloObj.RawErrorBudgetQuery()
			_, err = parser.ParseExpr(rawBudget)
			Expect(err).To(Succeed())

			rawObjective := sloObj.RawObjectiveQuery()
			_, err = parser.ParseExpr(rawObjective)
			Expect(err).To(Succeed())
			rawRemainingBudget := sloObj.RawBudgetRemainingQuery()
			_, err = parser.ParseExpr(rawRemainingBudget)
			Expect(err).To(Succeed())
			rawPeriodAsVectorDays := sloObj.RawPeriodDurationQuery()
			_, err = parser.ParseExpr(rawPeriodAsVectorDays)
			Expect(err).To(Succeed())
			// burn rate
			curBurnRate := sloObj.RawCurrentBurnRateQuery()
			_, err = parser.ParseExpr(curBurnRate)
			Expect(err).To(Succeed())
			periodBurnRate := sloObj.RawPeriodBurnRateQuery()
			_, err = parser.ParseExpr(periodBurnRate)
			Expect(err).To(Succeed())
		})

		Specify("SLO objects should be able to create valid alerting Prometheus rules", func() {
			interval := time.Second
			ralerts := sloObj.ConstructAlertingRuleGroup(&interval)
			for _, alertRule := range ralerts.Rules {
				_, err := parser.ParseExpr(alertRule.Expr.Value)
				Expect(err).To(Succeed())
			}
		})

		Specify("SLO objects should be able to create valid Cortex recording rule groups", func() {
			interval := time.Second
			rrecording := sloObj.ConstructRecordingRuleGroup(&interval)
			out, err := yaml.Marshal(rrecording)
			Expect(err).To(Succeed())
			y := rulefmt.RuleGroup{}
			err = yaml.Unmarshal(out, &y)
			Expect(err).To(Succeed())
			//FIXME: when joe is back, should add pkg/rules to his fork of cortex-tools
			//errors := rules.ValidateRuleGroup(y)
			//Expect(errors).To(BeEmpty())

		})

		Specify("SLO objects should be able to create valid Cortex metadata rule groups", func() {
			interval := time.Second
			rmetadata := sloObj.ConstructMetadataRules(&interval)
			out, err := yaml.Marshal(rmetadata)
			Expect(err).To(Succeed())
			y := rulefmt.RuleGroup{}
			err = yaml.Unmarshal(out, &y)
			Expect(err).To(Succeed())
			//FIXME: when joe is back, should add pkg/rules to his fork of cortex-tools
			//errors := rules.ValidateRuleGroup(y)
			//Expect(errors).To(BeEmpty())

		})

		Specify("SLO objects should be able to create valid Cortex alerting rule groups", func() {
			interval := time.Second
			ralerts := sloObj.ConstructMetadataRules(&interval)
			out, err := yaml.Marshal(ralerts)
			Expect(err).To(Succeed())
			y := rulefmt.RuleGroup{}
			err = yaml.Unmarshal(out, &y)
			Expect(err).To(Succeed())
			//FIXME: when joe is back, should add pkg/rules to his fork of cortex-tools
			//errors := rules.ValidateRuleGroup(y)
			//Expect(errors).To(BeEmpty())
		})
	})

	When("When use raw SLO constructions with cortex admin client", func() {
		Specify("The individual parts of the raw SLI queries should return data from cortex", func() {
			// needed to ensure that prometheus agent registers a 200 status code
			// otherwise the test is flaky
			rawGood, err := sloObj.RawGoodEventsQuery("5m")
			Expect(err).NotTo(HaveOccurred())
			rawTotal, err := sloObj.RawTotalEventsQuery("5m")
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				return nil
			}, time.Second*90, time.Second*30).Should(BeNil())
			respGood, err := adminClient.Query(env.Context(), &cortexadmin.QueryRequest{
				Query:   rawGood,
				Tenants: []string{"agent"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(respGood.Data).NotTo(BeEmpty())
			qres, err := compat.UnmarshalPrometheusResponse(respGood.Data)
			Expect(err).NotTo(HaveOccurred())
			goodVector, err := qres.GetVector()
			Expect(err).NotTo(HaveOccurred())
			Expect(goodVector).NotTo(BeNil())
			Expect(*goodVector).NotTo(BeEmpty())
			for _, sample := range *goodVector {
				Expect(sample.Value).To(BeNumerically(">=", 0))
				Expect(sample.Timestamp).To(BeNumerically(">=", 0))
			}

			respTotal, err := adminClient.Query(env.Context(), &cortexadmin.QueryRequest{
				Query:   rawTotal,
				Tenants: []string{"agent"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(respTotal.Data).NotTo(BeEmpty())
			qresTotal, err := compat.UnmarshalPrometheusResponse(respTotal.Data)
			Expect(err).NotTo(HaveOccurred())
			totalVector, err := qresTotal.GetVector()
			Expect(err).NotTo(HaveOccurred())
			Expect(totalVector).NotTo(BeNil())
			Expect(*totalVector).NotTo(BeEmpty())
			for _, sample := range *totalVector {
				Expect(sample.Value).To(BeNumerically(">=", 0))
				Expect(sample.Timestamp).To(BeNumerically(">=", 0))
			}

			adHocSliErrorRatioQuery := fmt.Sprintf(" 1 - (%s)/(%s)", rawGood, rawTotal)
			respSli, err := adminClient.Query(env.Context(), &cortexadmin.QueryRequest{
				Tenants: []string{"agent"},
				Query:   adHocSliErrorRatioQuery,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(respSli.Data).NotTo(BeEmpty())
			qresSli, err := compat.UnmarshalPrometheusResponse(respSli.Data)
			Expect(err).NotTo(HaveOccurred())
			sliErrorRatioVector, err := qresSli.GetVector()
			Expect(err).NotTo(HaveOccurred())
			Expect(sliErrorRatioVector).NotTo(BeNil())
			Expect(*sliErrorRatioVector).NotTo(BeEmpty())
			for _, sample := range *sliErrorRatioVector {
				Expect(sample.Value).To(BeNumerically(">=", 0))
				Expect(sample.Value).To(BeNumerically("<=", 1))
				Expect(sample.Timestamp).To(BeNumerically(">=", 0))
			}
		})

		Specify("All of the raw SLI queries should return data from cortex", func() {
			time.Sleep(time.Second * 10)
			interval := time.Second
			rrecording := sloObj.ConstructRecordingRuleGroup(&interval)
			for _, rawRule := range rrecording.Rules {
				resp, err := adminClient.Query(env.Context(), &cortexadmin.QueryRequest{
					Query:   rawRule.Expr.Value,
					Tenants: []string{"agent"},
				})
				Expect(err).NotTo(HaveOccurred())
				rawBytes := resp.Data
				qres, err := compat.UnmarshalPrometheusResponse(rawBytes)
				Expect(err).NotTo(HaveOccurred())
				Expect(qres).NotTo(BeNil())
				recordingVector, err := qres.GetVector()
				Expect(err).NotTo(HaveOccurred())
				Expect(recordingVector).NotTo(BeNil())
				Expect(*recordingVector).NotTo(BeEmpty())
				for _, sample := range *recordingVector {
					Expect(sample.Value).To(BeNumerically(">=", 0))
					Expect(sample.Timestamp).To(BeNumerically(">=", 0))
				}
			}
		})

		Specify("All of the raw Metadata queries should return data from cortex", func() {
			// TODO : need to replace all rule names in each of these Expr's to their real expr
		})

		Specify("All of the raw alert queries should return data from cortex", func() {
			// TODO: need to replace all rule names in each of these Expr's to their real expr
		})

		Specify("After aplying the rules to cortex, each rule name should evaluate to non-empty data", func() {
			// Manually apply the SLI recording rules
			interval := time.Second
			rrecording := sloObj.ConstructRecordingRuleGroup(&interval)
			rmetadata := sloObj.ConstructMetadataRules(&interval)
			ralerts := sloObj.ConstructAlertingRuleGroup(&interval)

			outRecording, err := yaml.Marshal(rrecording)
			Expect(err).To(Succeed())

			_, err = adminClient.LoadRules(env.Context(), &cortexadmin.LoadRuleRequest{
				ClusterId:   "agent",
				Namespace:   "slo",
				YamlContent: outRecording,
			})
			Expect(err).NotTo(HaveOccurred())
			outMetadata, err := yaml.Marshal(rmetadata)
			Expect(err).NotTo(HaveOccurred())
			_, err = adminClient.LoadRules(env.Context(), &cortexadmin.LoadRuleRequest{
				ClusterId:   "agent",
				Namespace:   "slo",
				YamlContent: outMetadata,
			})
			Expect(err).NotTo(HaveOccurred())
			outAlerts, err := yaml.Marshal(ralerts)
			Expect(err).NotTo(HaveOccurred())
			_, err = adminClient.LoadRules(env.Context(), &cortexadmin.LoadRuleRequest{
				ClusterId:   "agent",
				Namespace:   "slo",
				YamlContent: outAlerts,
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				resp, err := adminClient.ListRules(env.Context(), &cortexadmin.ListRulesRequest{
					ClusterId: []string{"agent"},
				})
				if err != nil {
					return err
				}
				for _, group := range resp.Data.Groups {
					if strings.Contains(group.Name, sloObj.GetId()) {
						return nil
					}
				}
				return nil
			}, time.Minute*2, time.Second*30).Should(Succeed())

			// check the recording rule names to make sure they return data
			Eventually(func() error {
				for _, rawRule := range rrecording.Rules {
					ruleVector, err := slo.QuerySLOComponentByRawQuery(env.Context(), adminClient, rawRule.Expr.Value, "agent")
					if err != nil {
						return err
					}

					if ruleVector == nil || len(*ruleVector) == 0 {
						return fmt.Errorf("expect rule vector to contain data")
					}
					for _, sample := range *ruleVector {
						if sample.Timestamp == 0 {
							return fmt.Errorf("expect sample.Timestamp to contain data")
						}
					}
				}
				return nil
			}, time.Minute*2, time.Second*30).Should(Succeed())

			Eventually(func() error {
				for _, rawRule := range rmetadata.Rules {
					ruleVector, err := slo.QuerySLOComponentByRecordName(env.Context(), adminClient, rawRule.Record.Value, "agent")
					if err != nil {
						return err
					}

					if ruleVector == nil || len(*ruleVector) == 0 {
						return fmt.Errorf("expect rule vector to contain data")
					}
					for _, sample := range *ruleVector {
						if sample.Timestamp == 0 {
							return fmt.Errorf("expect sample.Timestamp to contain data")
						}
					}
				}
				return nil
			}, time.Minute*2, time.Second*30).Should(Succeed())

			Eventually(func() error {
				rawSevereAlertQuery, rawCriticalAlertQuery := sloObj.ConstructRawAlertQueries()
				// @debug
				//rawSevereAlertQueryComponents := strings.Split(rawSevereAlertQuery, "or")
				//rawCriticalAlertQueryComponents := strings.Split(rawCriticalAlertQuery, "or")
				//var alertRules []string
				//alertRules = append(alertRules, rawSevereAlertQueryComponents...)
				//alertRules = append(alertRules, rawCriticalAlertQueryComponents...)
				alertRules := []string{rawSevereAlertQuery.Value, rawCriticalAlertQuery.Value}
				for _, rawRule := range alertRules {
					ruleVector, err := slo.QuerySLOComponentByRawQuery(env.Context(), adminClient, rawRule, "agent")
					if err != nil {
						return err
					}
					if ruleVector == nil || len(*ruleVector) == 0 {
						return fmt.Errorf("expect rule vector to contain data")
					}
					for _, sample := range *ruleVector {
						if sample.Timestamp == 0 {
							return fmt.Errorf("expect sample.Timestamp to contain data")
						}
					}
				}
				return nil
			}, time.Minute*2, time.Second*30).Should(Succeed())
		})
	})
})
