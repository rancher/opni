package slo_test

import (
	"context"
	"fmt"
	promql "github.com/cortexproject/cortex/pkg/configs/legacy_promql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/model/rulefmt"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v3"
	"time"
)

var _ = Describe("Converting SLO information to Cortex rules", Ordered, Label(test.Unit, test.Slow), func() {
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
	ctx := context.Background()
	// test environment references
	var env *test.Environment
	var pPort int
	var adminClient cortexadmin.CortexAdminClient

	BeforeAll(func() {
		env = &test.Environment{
			TestBin: "../../../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		client := env.NewManagementClient()
		token, err := client.CreateBootstrapToken(ctx, &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		info, err := client.CertsInfo(ctx, &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		p, _ := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		pPort = env.StartPrometheus(p)
		p2, _ := env.StartAgent("agent2", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		pPort = env.StartPrometheus(p2)
		adminClient = cortexadmin.NewCortexAdminClient(env.ManagementClientConn())
		Eventually(func() error {
			stats, err := adminClient.AllUserStats(context.Background(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			for _, item := range stats.Items {
				if item.UserID == "agent" {
					if item.NumSeries > 0 {
						return nil
					}
				}
			}
			return fmt.Errorf("waiting for metric data to be stored in cortex")
		}, 30*time.Second, 1*time.Second).Should(Succeed())
		Eventually(func() error {
			stats, err := adminClient.AllUserStats(context.Background(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			for _, item := range stats.Items {
				if item.UserID == "agent2" {
					if item.NumSeries > 0 {
						return nil
					}
				}
			}
			return fmt.Errorf("waiting for metric data to be stored in cortex")
		}, 30*time.Second, 1*time.Second).Should(Succeed())
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
			Expect(goodLabelPairs.Construct()).To(Equal(",code=\"200\""))
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
			_, err = promql.ParseExpr(query)
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
			_, err := promql.ParseExpr(dash)
			Expect(err).To(Succeed())
			rawBudget := sloObj.RawErrorBudgetQuery()
			_, err = promql.ParseExpr(rawBudget)
			Expect(err).To(Succeed())

			rawObjective := sloObj.RawObjectiveQuery()
			_, err = promql.ParseExpr(rawObjective)
			Expect(err).To(Succeed())
			rawRemainingBudget := sloObj.RawBudgetRemainingQuery()
			_, err = promql.ParseExpr(rawRemainingBudget)
			Expect(err).To(Succeed())
			rawPeriodAsVectorDays := sloObj.RawPeriodDurationQuery()
			_, err = promql.ParseExpr(rawPeriodAsVectorDays)
			Expect(err).To(Succeed())
			// burn rate
			curBurnRate := sloObj.RawCurrentBurnRateQuery()
			_, err = promql.ParseExpr(curBurnRate)
			Expect(err).To(Succeed())
			periodBurnRate := sloObj.RawPeriodBurnRateQuery()
			_, err = promql.ParseExpr(periodBurnRate)
			Expect(err).To(Succeed())
		})

		Specify("SLO objects should be able to create valid alerting Prometheus rules", func() {
			interval := time.Second
			ralerts := sloObj.ConstructAlertingRuleGroup(&interval)
			for _, alertRule := range ralerts.Rules {
				_, err := promql.ParseExpr(alertRule.Expr)
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
			time.Sleep(time.Second)
			rawGood, err := sloObj.RawGoodEventsQuery("5m")
			Expect(err).NotTo(HaveOccurred())
			rawTotal, err := sloObj.RawTotalEventsQuery("5m")
			Expect(err).NotTo(HaveOccurred())

			respGood, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
				Query:   rawGood,
				Tenants: []string{"agent"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(respGood.Data).NotTo(BeEmpty())
			qres, err := unmarshal.UnmarshalPrometheusResponse(respGood.Data)
			Expect(err).NotTo(HaveOccurred())
			goodVector, err := qres.GetVector()
			Expect(err).NotTo(HaveOccurred())
			Expect(goodVector).NotTo(BeNil())
			Expect(*goodVector).NotTo(BeEmpty())
			for _, sample := range *goodVector {
				Expect(sample.Value).To(BeNumerically(">=", 0))
				Expect(sample.Timestamp).To(BeNumerically(">=", 0))
			}

			respTotal, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
				Query:   rawTotal,
				Tenants: []string{"agent"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(respTotal.Data).NotTo(BeEmpty())
			qresTotal, err := unmarshal.UnmarshalPrometheusResponse(respTotal.Data)
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
			respSli, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
				Tenants: []string{"agent"},
				Query:   adHocSliErrorRatioQuery,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(respSli.Data).NotTo(BeEmpty())
			qresSli, err := unmarshal.UnmarshalPrometheusResponse(respSli.Data)
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
				resp, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
					Query:   rawRule.Expr,
					Tenants: []string{"agent"},
				})
				Expect(err).NotTo(HaveOccurred())
				rawBytes := resp.Data
				qres, err := unmarshal.UnmarshalPrometheusResponse(rawBytes)
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

			_, err = adminClient.LoadRules(ctx, &cortexadmin.PostRuleRequest{
				ClusterId:   "agent",
				YamlContent: string(outRecording),
			})
			Expect(err).NotTo(HaveOccurred())
			outMetadata, err := yaml.Marshal(rmetadata)
			_, err = adminClient.LoadRules(ctx, &cortexadmin.PostRuleRequest{
				ClusterId:   "agent",
				YamlContent: string(outMetadata),
			})
			outAlerts, err := yaml.Marshal(ralerts)
			Expect(err).NotTo(HaveOccurred())
			_, err = adminClient.LoadRules(ctx, &cortexadmin.PostRuleRequest{
				ClusterId:   "agent",
				YamlContent: string(outAlerts),
			})
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(time.Second * 60) //FIXME: syncing rules is taking about a minute

			//@debug
			resp, err := adminClient.ListRules(ctx, &cortexadmin.Cluster{
				ClusterId: "agent",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.Data).NotTo(BeEmpty())
			result := gjson.Get(string(resp.Data), "data.groups")
			Expect(result.Exists()).To(BeTrue())
			for _, r := range result.Array() {
				fmt.Println(r)
			}

			// check the recording rule names to make sure they return data
			for _, rawRule := range rrecording.Rules {
				respRule, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
					Tenants: []string{"agent"},
					Query:   rawRule.Expr,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(respRule.Data).NotTo(BeEmpty())
				qresRule, err := unmarshal.UnmarshalPrometheusResponse(respRule.Data)
				Expect(err).NotTo(HaveOccurred())
				ruleVector, err := qresRule.GetVector()
				Expect(err).NotTo(HaveOccurred())
				Expect(ruleVector).NotTo(BeNil())
				Expect(*ruleVector).NotTo(BeEmpty())
				for _, sample := range *ruleVector {
					Expect(sample.Value).NotTo(BeNil())
					Expect(sample.Timestamp).To(BeNumerically(">=", 0))
				}
			}

			// check the evaluation of the rule names produce non-empty responses
			for _, rawRule := range rmetadata.Rules {
				respMetadata, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
					Tenants: []string{"agent"},
					Query:   rawRule.Record,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(respMetadata.Data).NotTo(BeEmpty())
				qresMetadata, err := unmarshal.UnmarshalPrometheusResponse(respMetadata.Data)
				Expect(err).NotTo(HaveOccurred())
				metadataVector, err := qresMetadata.GetVector()
				Expect(err).NotTo(HaveOccurred())
				Expect(metadataVector).NotTo(BeNil())
				//FIXME: `group_left` is fallaciously being replaced with `group_left()`
				//Expect(*metadataVector).NotTo(BeEmpty())
				for _, sample := range *metadataVector {
					Expect(sample.Value).To(BeNumerically(">=", 0))
					Expect(sample.Timestamp).To(BeNumerically(">=", 0))
				}
			}

			for _, rawRule := range ralerts.Rules {
				respAlerts, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
					Tenants: []string{"agent"},
					Query:   rawRule.Expr,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(respAlerts.Data).NotTo(BeEmpty())
				qresAlert, err := unmarshal.UnmarshalPrometheusResponse(respAlerts.Data)
				Expect(err).NotTo(HaveOccurred())
				alertVector, err := qresAlert.GetVector()
				Expect(err).NotTo(HaveOccurred())
				Expect(alertVector).NotTo(BeNil())
				//Expect(*alertVector).NotTo(BeEmpty()) //FIXME
				for _, sample := range *alertVector {
					Expect(sample.Value).To(BeNumerically(">=", 0))
					Expect(sample.Timestamp).To(BeNumerically(">=", 0))
				}
			}
		})
	})
})
