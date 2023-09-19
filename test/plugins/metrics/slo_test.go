package metrics_test

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/golang/snappy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/slo/backend"
	"github.com/rancher/opni/pkg/slo/backend/metrics"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"
)

func init() {
	metrics.DefaultEvaluationInterval = 1 * time.Second
}

var _ = Describe("Prometheus SLOs", Ordered, Label("integration"), func() {
	var env *test.Environment
	var fingerprint string
	var adminClient cortexadmin.CortexAdminClient
	var sloGen *metrics.SLOGeneratorImpl
	var sloDatasource backend.SLODatasource
	var rw remote.WriteClient
	var sloRef *corev1.Reference
	BeforeAll(func() {
		env = &test.Environment{}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)
		mgmtClient := env.NewManagementClient()
		var err error
		sloGen, err = metrics.NewSLOGenerator(*backend.SLODataToStruct(&slov1.SLOData{
			Id:  "id1",
			SLO: util.ProtoClone(tempSLO),
		}))
		Expect(err).To(BeNil())

		certsInfo, err := mgmtClient.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())

		token, err := mgmtClient.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(1 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		agentPort, errC := env.StartAgent("agent", token, []string{fingerprint})
		Eventually(errC).Should(Receive(BeNil()))

		rw, err = remote.NewWriteClient("agent", &remote.ClientConfig{
			URL:     &config.URL{URL: util.Must(url.Parse(fmt.Sprintf("http://127.0.0.1:%d/api/agent/push", agentPort)))},
			Timeout: model.Duration(time.Second * 10),
		})
		Expect(err).NotTo(HaveOccurred())

		opsClient := cortexops.NewCortexOpsClient(env.ManagementClientConn())
		_, err = opsClient.ConfigureCluster(context.Background(), &cortexops.ClusterConfiguration{
			Mode: cortexops.DeploymentMode_AllInOne,
			Storage: &storagev1.StorageSpec{
				Backend: storagev1.Filesystem,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			clusters, err := mgmtClient.ListClusters(env.Context(), &managementv1.ListClustersRequest{})
			if err != nil {
				return err
			}
			if len(clusters.GetItems()) == 0 {
				return fmt.Errorf("no clusters found")
			}
			return nil
		}).Should(Succeed())

		resp, err := mgmtClient.InstallCapability(context.Background(), &managementv1.CapabilityInstallRequest{
			Name: "metrics",
			Target: &capabilityv1.InstallRequest{
				Cluster: &corev1.Reference{
					Id: "agent",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status).To(Equal(capabilityv1.InstallResponseStatus_Success))

		lg := logger.NewPluginLogger().Named("metrics")

		adminClient = cortexadmin.NewCortexAdminClient(env.ManagementClientConn())
		metricsProvider := metrics.NewProvider(lg.With("component", "slo"))
		metricsProvider.Initialize(adminClient)
		metricsStoreProvider := metrics.NewMetricsSLOStoreProvider(lg.With("component", "slo"))
		metricsStoreProvider.Initialize(adminClient)

		sloDatasource = backend.NewSLODatasource(
			metrics.NewMetricsSLOStore(
				metricsStoreProvider,
			),
			metrics.NewBackend(
				metricsProvider,
			),
			func(ctx context.Context, clusterId *corev1.Reference) error {
				cluster, err := mgmtClient.GetCluster(ctx, &corev1.Reference{
					Id: clusterId.Id,
				})
				if err != nil {
					return err
				}
				if !capabilities.Has(cluster, capabilities.Cluster(wellknown.CapabilityMetrics)) {
					return fmt.Errorf("requires metrics capability to be installed on cluster %s", cluster.GetId())
				}
				return nil
			},
		)
	})
	When("we use the prometheus slo generator", func() {
		Specify("cortex should be reachable", func() {
			Eventually(func() error {
				statsList, err := adminClient.AllUserStats(context.Background(), &emptypb.Empty{})
				if err != nil {
					return err
				}
				if len(statsList.Items) == 0 {
					return fmt.Errorf("no user stats")
				}
				if statsList.Items[0].NumSeries == 0 {
					return fmt.Errorf("no series")
				}
				return nil
			}, 60*time.Second, 100*time.Millisecond).Should(Succeed())
		})
		It("should construct loadable SLI info rule", func() {
			expectLoadRules(env.Context(), "info", adminClient, sloGen.Info())
		})

		It("should construct loadable SLI period rule ", func() {
			expectLoadRules(env.Context(), "period", adminClient, sloGen.Period())
		})

		It("should construct loadable SLI error budget rule", func() {
			expectLoadRules(env.Context(), "errbudget", adminClient, sloGen.ErrorBudget())
		})

		It("should construct loadable SLI objective rule", func() {
			expectLoadRules(env.Context(), "objective", adminClient, sloGen.Objective())
		})

		It("should construct loadable SLI SLI rule", func() {
			expectLoadRules(env.Context(), "objective", adminClient, sloGen.SLI(
				metrics.WindowMetadata{
					WindowDur: prommodel.Duration(time.Second * 5),
					Name:      "test",
				}))
		})

		It("should construct SLI current burn rate rule", func() {
			expectLoadRules(env.Context(), "currentBurnRate", adminClient, sloGen.CurrentBurnRate())
		})

		It("should construct SLI period burn rate rule", func() {
			expectLoadRules(env.Context(), "periodBurnRate", adminClient, sloGen.PeriodBurnRate())
		})

		It("should construct error budget remaining rule", func() {
			expectLoadRules(env.Context(), "errorBudgetRemaining", adminClient, sloGen.ErrorBudgetRemaining())
		})
	})

	When("we use the SLO monitoring datasource service discovery backend", func() {
		It("should remote write SLO metric data to cortex", func() {
			now := time.Now().UnixMilli()
			err := remoteWrite(env.Context(), rw, &prompb.WriteRequest{
				Timeseries: []prompb.TimeSeries{
					{
						Labels: []prompb.Label{
							{Name: "__name__", Value: "testmetric"},
							{Name: "job", Value: "testservice"},
							{Name: "a", Value: "a"},
						},
						Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
					},
				},
			})
			Expect(err).To(Succeed())
		})

		It("should list available services", func() {
			Eventually(func() error {
				svcList, err := sloDatasource.ListServices(env.Context(), &slov1.ListServicesRequest{
					Datasource: "monitoring",
					ClusterId:  "agent",
				})
				if err != nil {
					return err
				}
				if len(svcList.Items) == 0 {
					return fmt.Errorf("no services")
				}
				if !svcList.ContainsId("prometheus") {
					return fmt.Errorf("missing prometheus")
				}
				if !svcList.ContainsId("testservice") {
					return fmt.Errorf("missing testservice")
				}
				return nil
			}).Should(Succeed())
		})

		It("should list available metrics on a service", func() {
			Eventually(func() error {
				metricList, err := sloDatasource.ListMetrics(env.Context(), &slov1.ListMetricsRequest{
					Datasource: "monitoring",
					ClusterId:  "agent",
					ServiceId:  "testservice",
				})
				if err != nil {
					return err
				}

				if len(metricList.GroupNameToMetrics) == 0 {
					return fmt.Errorf("no metrics")
				}

				if !metricList.ContainsId("testmetric") {
					return fmt.Errorf("missing testmetric")
				}
				return nil
			}).Should(Succeed())

		})

		It("should list available events on a (metrics,service) pair", func() {
			Eventually(func() error {
				eventList, err := sloDatasource.ListEvents(env.Context(), &slov1.ListEventsRequest{
					Datasource: "monitoring",
					ClusterId:  "agent",
					ServiceId:  "testservice",
					MetricId:   "testmetric",
				})
				if err != nil {
					return err
				}
				if len(eventList.Items) == 0 {
					return fmt.Errorf("no events")
				}
				if !eventList.ContainsId("a") {
					return fmt.Errorf("missing event a")
				}
				return nil
			}).Should(Succeed())

		})
	})

	When("we create SLO data", func() {
		It("should write SLO data", func() {
			dur := prommodel.Duration(time.Hour * 2)
			err := writeSLO(
				env.Context(),
				rw,
				time.Duration(dur),
				"http_slo_response",
				"service",
				"code",
				"200",
				"500",
				99.0,
			)
			Expect(err)
		})

		It("should discover the previously written SLO data", func() {
			Eventually(func() error {
				svcList, err := sloDatasource.ListServices(env.Context(), &slov1.ListServicesRequest{
					Datasource: "monitoring",
					ClusterId:  "agent",
				})
				if err != nil {
					return err
				}
				if len(svcList.Items) == 0 {
					return fmt.Errorf("no services")
				}

				if !svcList.ContainsId("service") {
					return fmt.Errorf("missing service")
				}
				return nil
			}).Should(Succeed())

			Eventually(func() error {
				metricList, err := sloDatasource.ListMetrics(env.Context(), &slov1.ListMetricsRequest{
					Datasource: "monitoring",
					ClusterId:  "agent",
					ServiceId:  "service",
				})
				if err != nil {
					return err
				}

				if len(metricList.GroupNameToMetrics) == 0 {
					return fmt.Errorf("no metrics")
				}

				if !metricList.ContainsId("http_slo_response") {
					return fmt.Errorf("missing http_slo_response")
				}
				return nil
			}).Should(Succeed())

			Eventually(func() error {
				eventList, err := sloDatasource.ListEvents(env.Context(), &slov1.ListEventsRequest{
					Datasource: "monitoring",
					ClusterId:  "agent",
					ServiceId:  "service",
					MetricId:   "http_slo_response",
				})
				if err != nil {
					return err
				}
				if len(eventList.Items) == 0 {
					return fmt.Errorf("no events")
				}
				if !eventList.ContainsId("code") {
					return fmt.Errorf("missing event 'code'")
				}
				return nil
			}).Should(Succeed())
		})

		It("should be able to query the written data", func() {
			// asStr := func(samples []compat.Sample) string {
			// 	res := []string{}
			// 	for _, s := range samples {
			// 		res = append(res, fmt.Sprintf("V : %f, T : %d", s.Value, s.Timestamp))
			// 	}
			// 	return strings.Join(res, " -- ")
			// }
			resp, err := adminClient.QueryRange(env.Context(), &cortexadmin.QueryRangeRequest{
				Tenants: []string{
					"agent",
				},
				Query: "sum(http_slo_response{code=\"200\"})",
				Start: timestamppb.New(time.Now().Add(-time.Hour)),
				End:   timestamppb.New(time.Now()),
				Step:  durationpb.New(time.Second * 1),
			})
			Expect(err).To(Succeed())
			Expect(resp).NotTo(BeNil())
			qr, err := compat.UnmarshalPrometheusResponse(resp.Data)
			Expect(err).To(Succeed())
			samples := qr.LinearSamples()
			// GinkgoWriter.Write([]byte(asStr(samples)))
			Expect(samples).NotTo(HaveLen(0))

			resp2, err := adminClient.QueryRange(env.Context(), &cortexadmin.QueryRangeRequest{
				Tenants: []string{
					"agent",
				},
				Query: "sum(http_slo_response{code=~\"200|500\"})",
				Start: timestamppb.New(time.Now().Add(-time.Hour)),
				End:   timestamppb.New(time.Now()),
				Step:  durationpb.New(time.Second * 1),
			})
			Expect(err).To(Succeed())
			Expect(resp2).NotTo(BeNil())
			qr2, err := compat.UnmarshalPrometheusResponse(resp2.Data)
			Expect(err).To(Succeed())
			samples2 := qr2.LinearSamples()
			// GinkgoWriter.Write([]byte(asStr(samples2)))
			Expect(samples2).NotTo(HaveLen(0))
			Eventually(func() error {
				resp3, err := adminClient.QueryRange(env.Context(), &cortexadmin.QueryRangeRequest{
					Tenants: []string{
						"agent",
					},
					Query: "sum(rate(http_slo_response{code=~\"200\"}[1m])) /  sum(rate(http_slo_response{code=~\"500|200\"}[1m]))",
					Start: timestamppb.New(time.Now().Add(-time.Hour)),
					End:   timestamppb.New(time.Now()),
					Step:  durationpb.New(time.Second * 1),
				})
				if err != nil {
					return err
				}
				if resp3 == nil {
					return fmt.Errorf("nil response")
				}
				qr3, err := compat.UnmarshalPrometheusResponse(resp3.Data)
				if err != nil {
					return err
				}
				samples3 := qr3.LinearSamples()
				// GinkgoWriter.Write([]byte(asStr(samples3)))
				if len(samples3) == 0 {
					return fmt.Errorf("no samples")
				}
				return nil
			}).Should(Succeed())
		})

		It("should create an SLO based on the written data", func() {
			req := &slov1.CreateSLORequest{
				Slo: util.ProtoClone(metricsSlo),
			}
			plotVec, err := sloDatasource.Preview(env.Context(), req)
			Expect(err).To(Succeed())
			Expect(plotVec).NotTo(BeNil())
			Expect(plotVec.PlotVector.Items).NotTo(HaveLen(0))

			for _, item := range plotVec.PlotVector.Items {
				Expect(item).NotTo(BeNil())
				Expect(item.Sli).To(BeNumerically(">", float64(0)))
				Expect(item.Sli).To(BeNumerically("<=", float64(100)))
			}

			ref, err := sloDatasource.Create(env.Context(), req)
			sloRef = ref
			Expect(err).To(Succeed())
			Expect(ref).NotTo(BeNil())
			Expect(ref.Id).NotTo(BeEmpty())

			By("verifying it eventually has a status")
			Eventually(func() slov1.SLOStatusState {
				status, err := sloDatasource.Status(env.Context(), &slov1.SLOData{
					Id:  ref.Id,
					SLO: req.Slo,
				})

				if err != nil {
					return slov1.SLOStatusState_InProgress
				}

				return status.State
			}, time.Second*5, time.Millisecond*100).Should(
				Equal(slov1.SLOStatusState_Ok),
			)

			By("verifying after update to a higher target, it should have breached the error budget")
			incoming := &slov1.SLOData{
				Id:  ref.Id,
				SLO: util.ProtoClone(metricsSlo),
			}
			existing := &slov1.SLOData{
				Id:  ref.Id,
				SLO: util.ProtoClone(metricsSlo),
			}
			incoming.SLO.Target.Value = 99.9999
			_, err = sloDatasource.Update(env.Context(), incoming, existing)
			Expect(err).To(Succeed())

			Eventually(func() slov1.SLOStatusState {
				status, err := sloDatasource.Status(env.Context(), incoming)
				if err != nil {
					return slov1.SLOStatusState_InProgress
				}
				return status.State
			}).Should(Equal(slov1.SLOStatusState_Breaching))

			// TODO : this is wrong, might need to tweak alert rules
			// By("verifying updates of a target that should pick up burn rate")

			// incoming2 := util.ProtoClone(incoming)
			// incoming2.SLO.Target.Value = 99.5
			// _, err = sloDatasource.Update(env.Context(), incoming2, incoming)
			// Expect(err).To(Succeed())

			// Eventually(func() slov1.SLOStatusState {
			// 	status, err := sloDatasource.Status(env.Context(), incoming2)
			// 	if err != nil {
			// 		return slov1.SLOStatusState_InProgress
			// 	}
			// 	return status.State
			// }).Should(Equal(slov1.SLOStatusState_Warning))

		})

		It("should be able to delete a metrics SLO", func() {
			Expect(sloRef).NotTo(BeNil())
			Expect(sloRef.Id).NotTo(BeEmpty())
			Expect(sloDatasource.Delete(env.Context(), &slov1.SLOData{
				Id:  sloRef.Id,
				SLO: util.ProtoClone(metricsSlo),
			})).To(Succeed())

			Eventually(func() error {
				_, err := adminClient.GetRule(env.Context(), &cortexadmin.GetRuleRequest{
					ClusterId: "agent",
					Namespace: "slo",
					GroupName: sloRef.Id,
				})
				if err == nil {
					return fmt.Errorf("rule still exists")
				}
				if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
					return nil
				} else if ok {
					return fmt.Errorf("unexpected status code %d", st.Code())
				} else if err != nil {
					return err
				}
				return fmt.Errorf("unreachable code?")
			}).Should(Succeed())
		})
	})
})

func expectLoadRules(
	ctx context.Context,
	ruleGroupName string,
	adminClient cortexadmin.CortexAdminClient,
	gen metrics.MetricGenerator,
) {
	periodRule, err := gen.Rule()
	Expect(err).To(Succeed())

	periodGroup := rulefmt.RuleGroup{
		Name:     ruleGroupName,
		Interval: prommodel.Duration(1 * time.Second),
		Rules:    []rulefmt.RuleNode{periodRule},
	}

	periodData, err := yaml.Marshal(periodGroup)
	Expect(err).To(Succeed())

	_, err = adminClient.LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   "agent",
		Namespace:   "test-slo",
		YamlContent: periodData,
	})
	Expect(err).To(Succeed())
}

func remoteWrite(ctx context.Context, rw remote.WriteClient, req *prompb.WriteRequest) error {
	wrData, err := req.Marshal()
	Expect(err).NotTo(HaveOccurred())
	compressed := snappy.Encode(nil, wrData)

	return rw.Store(ctx, compressed)
}

func writeSLO(
	ctx context.Context,
	rw remote.WriteClient,
	period time.Duration,
	metricName, serviceName, eventName, goodEvent, totalEvent string,
	expectedSLI float64,
) error {
	if expectedSLI > 100 && expectedSLI < 0 {
		return fmt.Errorf("expected SLI must be between 0 and 100")
	}

	wreq := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{},
	}

	goodEvents := prompb.TimeSeries{
		Labels: []prompb.Label{
			{Name: "__name__", Value: metricName},
			{Name: "job", Value: serviceName},
			{Name: eventName, Value: goodEvent},
		},
		Samples: []prompb.Sample{},
	}
	totalEvents := prompb.TimeSeries{
		Labels: []prompb.Label{
			{Name: "__name__", Value: metricName},
			{Name: "job", Value: serviceName},
			{Name: eventName, Value: totalEvent},
		},
	}

	now := time.Now()
	start := now.Add(-period)

	// 4 zeroes
	precision := 1000
	iExpectedSLI := precision * int(expectedSLI)
	iMax := 100 * precision
	goodVal := 1
	totalVal := 1

	// write every 250ms
	for i := 0; i < int(period.Milliseconds()); i += 500 {
		ts := start.Add(time.Duration(i) * time.Millisecond)
		cur := i % iMax
		if cur < iMax-iExpectedSLI {
			totalVal++
		} else {
			goodVal++
		}
		goodEvents.Samples = append(goodEvents.Samples, prompb.Sample{
			Value:     float64(goodVal),
			Timestamp: ts.UnixMilli(),
		})
		totalEvents.Samples = append(totalEvents.Samples, prompb.Sample{
			Value:     float64(totalVal),
			Timestamp: ts.UnixMilli(),
		})
	}

	if len(goodEvents.Samples) == 0 {
		panic("bug : no good events")
	}

	if len(totalEvents.Samples) == 0 {
		panic("bug : no total events")
	}

	wreq.Timeseries = append(wreq.Timeseries, goodEvents)
	wreq.Timeseries = append(wreq.Timeseries, totalEvents)
	return remoteWrite(ctx, rw, wreq)
}

var (
	tempSLO = &slov1.ServiceLevelObjective{
		Name:           "test-slo-success",
		ClusterId:      "agent",
		ServiceId:      "testservice",
		GoodMetricName: "testmetric",
		GoodEvents: []*slov1.Event{
			{
				Key:  "code",
				Vals: []string{"200"},
			},
		},
		TotalMetricName:   "testmetric",
		TotalEvents:       []*slov1.Event{},
		Target:            &slov1.Target{Value: 99.9},
		SloPeriod:         "30d",
		BudgetingInterval: durationpb.New(time.Minute * 5),
		Datasource:        "monitoring",
	}

	metricsSlo = &slov1.ServiceLevelObjective{
		Name:           "some-opaque-name",
		Datasource:     "monitoring",
		ClusterId:      "agent",
		ServiceId:      "service",
		GoodMetricName: "http_slo_response",
		GoodEvents: []*slov1.Event{
			{
				Key:  "code",
				Vals: []string{"200"},
			},
		},
		TotalMetricName: "http_slo_response",
		TotalEvents: []*slov1.Event{
			{
				Key:  "code",
				Vals: []string{"500"},
			},
		},
		SloPeriod:         "2h",
		Target:            &slov1.Target{Value: 90.0},
		BudgetingInterval: durationpb.New(time.Second * 1),
	}
)
