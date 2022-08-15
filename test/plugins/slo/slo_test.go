package plugins_test

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"math"
	"net/http"
	"time"
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
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/%s/good", instrumentationServerPort, metricName))
			Expect(err).To(Succeed())
			Expect(resp.StatusCode).To(Equal(200))
		}()
	}
}

func simulateBadEvents(metricName string, instrumentationServerPort int, numEvents int) {

	for i := 0; i < numEvents; i++ {
		go func() {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/%s/bad", instrumentationServerPort, metricName))
			Expect(err).To(Succeed())
			Expect(resp.StatusCode).To(Equal(200))
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

var _ = Describe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	// test environment references
	var env *test.Environment
	var sloClient sloapi.SLOClient
	var adminClient cortexadmin.CortexAdminClient
	// downstream server ports
	var pPort int
	var pPort2 int

	var createdSlos []*corev1.Reference

	BeforeAll(func() {
		env = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		client := env.NewManagementClient()
		token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		info, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		p, _ := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		pPort = env.StartPrometheus(p)
		p2, _ := env.StartAgent("agent2", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		pPort2 = env.StartPrometheus(p2)

		Expect(pPort != 0 && pPort2 != 0).To(BeTrue())
		sloClient = sloapi.NewSLOClient(env.ManagementClientConn())
		adminClient = cortexadmin.NewCortexAdminClient(env.ManagementClientConn())
		fmt.Println("Before all done")
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
		fmt.Println(createdSlos)
		time.Sleep(time.Second * 10)
	})

	When("The SLO api is provided invalid input", func() {

	})

	When("The SLO plugin starts, service discovery ", func() {
		It("should be able to discover services from downstream", func() {
			expectedNames := []string{"prometheus"}
			resp, err := sloClient.ListServices(ctx, &sloapi.ListServicesRequest{
				Datasource: shared.MonitoringDatasource,
				ClusterdId: "agent",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(HaveLen(1))

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
				ClusterdId: "agent2",
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
			Expect(resp.Items).NotTo(HaveLen(0))
			for _, m := range resp.Items {
				Expect(m.GetId()).NotTo(Equal(""))
				//FIXME: when the metric metadata api works, check for metadata
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
						"500",
						"503",
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

		It("Should update SLOs", func() {
			resp, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(HaveLen(2))
			updateData := resp.Items[1]
			updateData.SLO.Name = "test-slo-updated"
			updateData.SLO.ClusterId = "agent2"
			item, err := sloClient.UpdateSLO(ctx, updateData)
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(item)
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
			}, time.Minute*2, time.Second*30).Should(Equal(sloapi.SLOStatusState_Ok))
		})

		It("Should preview SLOs in a raw data format", func() {
			//TODO
		})
	})
})
