package plugins_test

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/slo/query"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func canReachInstrumentationMetrics(instrumentationServerPort int) bool {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", instrumentationServerPort))
	if err != nil {
		panic(err)
	}
	return resp.StatusCode == 200
}

func simulateGoodEvents(metricName string, instrumentationServerPort int, numEvents int) {
	for i := 0; i < numEvents; i++ {
		go func() {
			client := &http.Client{
				Transport: &http.Transport{},
			}
			req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/%s/good", instrumentationServerPort, metricName), nil)
			req.Close = true
			resp, err := client.Do(req)
			if err != nil {
				panic(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				panic(resp.StatusCode)
			}
		}()
	}
}

func simulateBadEvents(metricName string, instrumentationServerPort int, numEvents int) {

	for i := 0; i < numEvents; i++ {
		go func() {
			client := &http.Client{
				Transport: &http.Transport{},
			}
			req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/%s/bad", instrumentationServerPort, metricName), nil)
			resp, err := client.Do(req)
			if err != nil {
				panic(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				panic(resp.StatusCode)
			}
		}()
	}
}

// populate instrumentation server with good events
func simulateGoodStatus(metricName string, instrumentationServerPort int, numEvents int) (goodEventsCount int) {
	simulateGoodEvents(metricName, instrumentationServerPort, numEvents)
	return numEvents
}

// populate instrumentation server with enough bad events to trigger an alerts,
// but maintain the current objective so we don't trigger a breaching status
//
// @warning : assumes slo objective is 0 <= x <= 100
func simulateAlertingStatus(
	metricName string,
	instrumentationServerPort int,
	numExistingGoodEvents int,
	sloObjective float64) (currentEventRatio float64, numTotalEvents int) {

	// solve an equation for the good events / total events ratio >= sloObjective
	// but gets as close as possible to the sloObjective

	if numExistingGoodEvents >= 0 {
		panic("Need existing number of good events to easily calculate an alerting status")
	}

	totalEventCount := int((sloObjective * 100) * 100)
	remainingToAssign := totalEventCount - numExistingGoodEvents
	// round up to ensure never triggering an SLO breach
	goodEventsNum := int(math.Ceil(float64(remainingToAssign) * (sloObjective / 100)))
	badEventsNum := remainingToAssign - goodEventsNum
	simulateGoodEvents(metricName, instrumentationServerPort, goodEventsNum)
	simulateBadEvents(metricName, instrumentationServerPort, badEventsNum)
	// return base 100 ratio of good events to total events
	return float64(
			(numExistingGoodEvents+goodEventsNum)/(numExistingGoodEvents+goodEventsNum+badEventsNum)) * 100,
		numExistingGoodEvents + goodEventsNum + badEventsNum
}

/*
Simulate a breaching status, by adding enough bad events to fail the objective
*/
func simulateBreachingStatus(metricName string, instrumentationServerPort int,
	sloObjective float64, curEventRatio float64, totalEvents int) {
	if curEventRatio >= sloObjective {
		panic("Expected to at least be in a good/alerting status to simulate a breaching status")
	}
	ratioToBreach := sloObjective - curEventRatio
	numEventsToBreach := int(math.Ceil(float64(totalEvents)*(ratioToBreach/100))) + 500 /* for good measure :) */
	simulateBadEvents(metricName, instrumentationServerPort, numEventsToBreach)
}

var _ = XDescribe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label("integration", "slow"), func() {
	ctx := context.Background()
	// test environment references
	var env *test.Environment
	var sloClient sloapi.SLOClient
	var adminClient cortexadmin.CortexAdminClient
	var client managementv1.ManagementClient
	// downstream server ports
	var instrumentationPort int
	var done chan struct{}
	var token *corev1.BootstrapToken
	var info *managementv1.CertsInfoResponse

	BeforeAll(func() {
		env = &test.Environment{}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		opsClient := cortexops.NewCortexOpsClient(env.ManagementClientConn())
		_, err := opsClient.ConfigureCluster(context.Background(), &cortexops.ClusterConfiguration{
			Mode: cortexops.DeploymentMode_AllInOne,
			Storage: &storagev1.StorageSpec{
				Backend: storagev1.Filesystem,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		client = env.NewManagementClient()
		token, err = client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		info, err = client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		instrumentationPort, done = env.StartInstrumentationServer(ctx)
		DeferCleanup(func() {
			done <- struct{}{}
		})
		_, errC := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		Eventually(errC).Should(Receive(BeNil()))
		env.SetPrometheusNodeConfigOverride("agent", test.NewOverridePrometheusConfig(
			"slo/prometheus/config.yaml",
			[]test.PrometheusJob{
				{
					JobName:    query.MockTestServerName,
					ScrapePort: instrumentationPort,
				},
			}),
		)

		_, errC = env.StartAgent("agent2", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		Eventually(errC).Should(Receive(BeNil()))

		client.InstallCapability(context.Background(), &managementv1.CapabilityInstallRequest{
			Name:   wellknown.CapabilityMetrics,
			Target: &v1.InstallRequest{Cluster: &corev1.Reference{Id: "agent"}},
		})
		client.InstallCapability(context.Background(), &managementv1.CapabilityInstallRequest{
			Name:   wellknown.CapabilityMetrics,
			Target: &v1.InstallRequest{Cluster: &corev1.Reference{Id: "agent2"}},
		})

		sloClient = sloapi.NewSLOClient(env.ManagementClientConn())
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
		time.Sleep(time.Second * 10)
	})

	When("The instrumentation server starts", func() {
		It("Should simulate events", func() {
			Expect(instrumentationPort).NotTo(Equal(0))
			simulateGoodEvents("http-availability", instrumentationPort, 1000)
			simulateBadEvents("http-availability", instrumentationPort, 10000)
		})
	})

	When("The SLO plugin starts, service discovery ", func() {
		It("should be able to discover services from downstream", func() {
			expectedNames := []string{"prometheus"}
			resp, err := sloClient.ListServices(ctx, &sloapi.ListServicesRequest{
				Datasource: shared.MonitoringDatasource,
				ClusterId:  "agent",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).NotTo(HaveLen(0))

			for _, name := range expectedNames {
				found := false
				for _, svc := range resp.GetItems() {
					if svc.GetServiceId() == name {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue())
			}
			resp2, err := sloClient.ListServices(ctx, &sloapi.ListServicesRequest{
				Datasource: shared.MonitoringDatasource,
				ClusterId:  "agent2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp2.Items).To(HaveLen(1))
			for _, name := range expectedNames {
				found := false
				for _, svc := range resp2.GetItems() {
					if svc.GetServiceId() == name {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue())
			}
		})

		It("should be able to discover metrics from downstream", func() {
			resp, err := sloClient.ListMetrics(ctx, &sloapi.ListMetricsRequest{
				Datasource: shared.MonitoringDatasource,
				ClusterId:  "agent",
				ServiceId:  "prometheus",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.GroupNameToMetrics).NotTo(HaveLen(0))
			for _, m := range resp.GroupNameToMetrics {
				for _, metric := range m.Items {
					Expect(metric.GetId()).NotTo(Equal(""))
				}

				// FIXME: when the metric metadata api works, check for metadata
			}
		})

		It("Should be able to discover events from downstream", func() {
			resp, err := sloClient.ListEvents(ctx, &sloapi.ListEventsRequest{
				Datasource: shared.MonitoringDatasource,
				ServiceId:  "prometheus",
				ClusterId:  "agent",
				MetricId:   "prometheus_http_requests_total",
			})
			expected := []*sloapi.Event{
				{
					Key: "code",
					Vals: []string{
						"200",
					},
				},
				{
					Key: "handler",
					Vals: []string{
						"/-/ready",
					},
				},
			}

			Expect(err).NotTo(HaveOccurred())
			for _, ex := range expected {
				foundKey := false
				for _, output := range resp.Items {
					if output.Key == ex.Key {
						foundKey = true
						for _, exval := range ex.Vals {
							foundVal := false
							for _, outval := range output.Vals {
								if exval == outval {
									foundVal = true
									break
								}
							}
							Expect(foundVal).To(BeTrue())
						}
						break
					}
				}
				Expect(foundKey).To(BeTrue())
			}
		})
	})

	When("CRUDing SLOs", func() {
		It("Should error on invalid cluster id", func() {
			_, err := sloClient.CreateSLO(ctx, &sloapi.CreateSLORequest{
				Slo: &sloapi.ServiceLevelObjective{
					Name:            "testslo",
					Datasource:      shared.MonitoringDatasource,
					ClusterId:       "asdakjsdhkjashdjkahsdkjhakjsdhkjashdkj",
					ServiceId:       "prometheus",
					GoodMetricName:  "prometheus_http_requests_total",
					TotalMetricName: "prometheus_http_requests_total",
					GoodEvents: []*sloapi.Event{
						{
							Key: "code",
							Vals: []string{
								"200",
							},
						},
					},
					TotalEvents: []*sloapi.Event{
						{
							Key: "code",
							Vals: []string{
								"200",
								"500",
								"503",
							},
						},
					},
					SloPeriod:         "30d",
					BudgetingInterval: durationpb.New(time.Minute * 5),
					Target: &sloapi.Target{
						Value: 99.99,
					},
				},
			})
			Expect(err).To(HaveOccurred())
		})

		It("Should create SLOs", func() {
			_, err := sloClient.CreateSLO(ctx, &sloapi.CreateSLORequest{
				Slo: &sloapi.ServiceLevelObjective{
					Name:            "testslo",
					Datasource:      shared.MonitoringDatasource,
					ClusterId:       "agent",
					ServiceId:       "prometheus",
					GoodMetricName:  "prometheus_http_requests_total",
					TotalMetricName: "prometheus_http_requests_total",
					GoodEvents: []*sloapi.Event{
						{
							Key: "code",
							Vals: []string{
								"200",
							},
						},
					},
					TotalEvents: []*sloapi.Event{
						{
							Key: "code",
							Vals: []string{
								"200",
								"500",
								"503",
							},
						},
					},
					SloPeriod:         "30d",
					BudgetingInterval: durationpb.New(time.Minute * 5),
					Target: &sloapi.Target{
						Value: 99.99,
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should clone SLO", func() {
			resp, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(HaveLen(1))
			_, err = sloClient.CloneSLO(ctx, &corev1.Reference{Id: resp.Items[0].Id})
			Expect(err).NotTo(HaveOccurred())
			respAfter, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(respAfter.Items).To(HaveLen(2))
		})

		It("Should error on update if provided a new cluster id", func() {
			resp, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(HaveLen(2))
			updateData := resp.Items[1]
			updateData.SLO.Name = "test-slo-updated"
			updateData.SLO.ClusterId = "asdkjasjkdhkajshdjkahsdjkhajkshdkjahsdjkhasjkdhkjasd"
			_, err = sloClient.UpdateSLO(ctx, updateData)
			Expect(err).To(HaveOccurred())

		})

		It("Should update SLOs", func() {
			resp, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(HaveLen(2))
			updateData := resp.Items[1]
			updateData.SLO.Name = "test-slo-updated"
			updateData.SLO.ClusterId = "agent2"
			_, err = sloClient.UpdateSLO(ctx, updateData)
			Expect(err).NotTo(HaveOccurred())
			respAfter, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(respAfter.Items).To(HaveLen(2))
		})

		It("Should Get SLOs", func() {
			resp, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(HaveLen(2))
			_, err = sloClient.GetSLO(ctx, &corev1.Reference{Id: resp.Items[1].Id})
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should delete SLOs", func() {
			resp, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(HaveLen(2))
			_, err = sloClient.DeleteSLO(ctx, &corev1.Reference{Id: resp.Items[1].Id})
			Expect(err).NotTo(HaveOccurred())
			respAfter, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(respAfter.Items).To(HaveLen(1))
		})

		It("Should get status for SLOs", func() {
			respList, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(respList.Items).To(HaveLen(1))
			resp, err := sloClient.Status(ctx, &corev1.Reference{Id: respList.Items[0].Id})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.State).To(Equal(sloapi.SLOStatusState_Creating))
			Eventually(func() sloapi.SLOStatusState {
				resp, err := sloClient.Status(ctx, &corev1.Reference{Id: respList.Items[0].Id})
				Expect(err).NotTo(HaveOccurred())
				return resp.State
			}, time.Minute*3, time.Second*1).Should(BeElementOf(sloapi.SLOStatusState_Ok, sloapi.SLOStatusState_PartialDataOk))
		})

		It("Should preview SLOs in a raw data format", func() {
			respList, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(respList.Items).To(HaveLen(1))
			resp, err := sloClient.Preview(ctx, &sloapi.CreateSLORequest{
				Slo: &sloapi.ServiceLevelObjective{
					Name:            "testslo",
					Datasource:      shared.MonitoringDatasource,
					ClusterId:       "agent",
					ServiceId:       "prometheus",
					GoodMetricName:  "prometheus_http_requests_total",
					TotalMetricName: "prometheus_http_requests_total",
					GoodEvents: []*sloapi.Event{
						{
							Key: "code",
							Vals: []string{
								"200",
							},
						},
					},
					TotalEvents: []*sloapi.Event{
						{
							Key: "code",
							Vals: []string{
								"200",
								"500",
								"503",
							},
						},
					},
					SloPeriod:         "30d",
					BudgetingInterval: durationpb.New(time.Minute * 5),
					Target: &sloapi.Target{
						Value: 99.99,
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.PlotVector.Items).NotTo(BeEmpty())
			hasData := false
			for _, y := range resp.PlotVector.Items {
				if y.Sli > 0 {
					hasData = true
					break
				}
			}
			Expect(hasData).To(BeTrue())

			respMyServer, err := sloClient.Preview(ctx, &sloapi.CreateSLORequest{
				Slo: &sloapi.ServiceLevelObjective{
					Name:            "testslo",
					Datasource:      shared.MonitoringDatasource,
					ClusterId:       "agent",
					ServiceId:       "MyServer",
					GoodMetricName:  "http_request_duration_seconds_count",
					TotalMetricName: "http_request_duration_seconds_count",
					GoodEvents: []*sloapi.Event{
						{
							Key: "code",
							Vals: []string{
								"200",
							},
						},
					},
					TotalEvents: []*sloapi.Event{
						{
							Key: "code",
							Vals: []string{
								"200",
								"500",
								"501",
								"502",
								"503",
							},
						},
					},
					SloPeriod:         "30d",
					BudgetingInterval: durationpb.New(time.Minute * 5),
					Target: &sloapi.Target{
						Value: 99.99,
					},
				},
			})
			Expect(err).To(Succeed())
			Expect(respMyServer.PlotVector.Items).NotTo(BeEmpty())
			Expect(respMyServer.PlotVector.Items[len(respMyServer.PlotVector.Items)-1].Sli).To(BeNumerically(">", 0.0))
			Expect(respMyServer.PlotVector.Items[len(respMyServer.PlotVector.Items)-1].Sli).To(BeNumerically("<", 100.0))
			Expect(respMyServer.PlotVector.Windows).To(HaveLen(2))
			// both alerts should be firing based on the number of bad events detected
			hasSevere, hasCritical := false, false
			for _, w := range respMyServer.PlotVector.Windows {
				if w.Severity == "severe" {
					hasSevere = true
				}
				if w.Severity == "critical" {
					hasCritical = true
				}
			}
			Expect(hasSevere).To(BeTrue())
			Expect(hasCritical).To(BeTrue())
		})

		Specify("Creating an SLO for the service that should be alerting", func() {
			failingSloId, err := sloClient.CreateSLO(ctx, &sloapi.CreateSLORequest{
				Slo: &sloapi.ServiceLevelObjective{
					Name:            "testslo",
					Datasource:      shared.MonitoringDatasource,
					ClusterId:       "agent",
					ServiceId:       "MyServer",
					GoodMetricName:  "http_request_duration_seconds_count",
					TotalMetricName: "http_request_duration_seconds_count",
					GoodEvents: []*sloapi.Event{
						{
							Key: "code",
							Vals: []string{
								"200",
							},
						},
					},
					TotalEvents: []*sloapi.Event{
						{
							Key: "code",
							Vals: []string{
								"200",
								"500",
								"501",
								"502",
								"503",
							},
						},
					},
					SloPeriod:         "30d",
					BudgetingInterval: durationpb.New(time.Minute * 5),
					Target: &sloapi.Target{
						Value: 99.99,
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() sloapi.SLOStatusState {
				resp, err := sloClient.Status(ctx, &corev1.Reference{Id: failingSloId.Id})
				Expect(err).NotTo(HaveOccurred())
				return resp.State
			}, time.Minute*3, time.Second*1).Should(BeElementOf(sloapi.SLOStatusState_Warning, sloapi.SLOStatusState_Breaching))
		})

		Specify("Multi Cluster Clone should clone to valid targets", func() {
			slos, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).To(Succeed())
			Expect(slos.Items).NotTo(HaveLen(0))
			toCloneId := slos.Items[0].Id
			var agentsCancel []context.CancelFunc
			var agentClusterIds []*corev1.Reference
			for i := 0; i < 10; i++ {
				id := fmt.Sprintf("agent-%d", i)
				ctxCa, cancelFunc := context.WithCancel(ctx)
				_, errC := env.StartAgent(
					id,
					token,
					[]string{info.Chain[len(info.Chain)-1].Fingerprint},
					test.WithContext(ctxCa),
				)
				env.SetPrometheusNodeConfigOverride(id, test.NewOverridePrometheusConfig(
					"slo/prometheus/config.yaml",
					[]test.PrometheusJob{
						{
							JobName:    query.MockTestServerName,
							ScrapePort: instrumentationPort,
						},
					}),
				)
				Eventually(errC).Should(Receive(BeNil()))

				_, err := client.InstallCapability(ctx, &managementv1.CapabilityInstallRequest{
					Name:   wellknown.CapabilityMetrics,
					Target: &v1.InstallRequest{Cluster: &corev1.Reference{Id: id}},
				})
				Expect(err).NotTo(HaveOccurred())

				agentsCancel = append(agentsCancel, cancelFunc)
				agentClusterIds = append(agentClusterIds, &corev1.Reference{Id: id})
			}

			Eventually(func() error {
				stats, err := adminClient.AllUserStats(context.Background(), &emptypb.Empty{})
				if err != nil {
					return err
				}
				for _, expectedItem := range agentClusterIds {
					found := false
					for _, item := range stats.Items {
						if item.UserID == expectedItem.Id {
							if item.NumSeries > 0 {
								found = true
							}
						}
					}
					if !found {
						return fmt.Errorf("Waiting for metric data for cluster %s", expectedItem.Id)
					}
				}
				return nil

			}, 30*time.Second, 1*time.Second).Should(Succeed())

			failures, err := sloClient.CloneToClusters(ctx, &sloapi.MultiClusterSLO{
				CloneId:  &corev1.Reference{Id: toCloneId},
				Clusters: agentClusterIds,
			})
			Expect(err).To(Succeed())
			Expect(failures.Failures).To(BeEmpty())

			for _, cancelFunc := range agentsCancel {
				cancelFunc()
			}
		})
	})
})
