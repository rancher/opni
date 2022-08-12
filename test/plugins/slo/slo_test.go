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
		//XIt("Should create valid SLOs", func() {
		//	inputSLO := &sloapi.ServiceLevelObjective{
		//		Name:              "test-slo",
		//		Datasource:        shared.MonitoringDatasource,
		//		MetricName:        "http-availability",
		//		MonitorWindow:     "30d",                           // one of 30d, 28, 7d
		//		BudgetingInterval: durationpb.New(time.Minute * 5), // between 5m and 1h
		//		Labels:            []*sloapi.Label{{Name: "env"}, {Name: "dev"}},
		//		Target:            &sloapi.Target{Value: 99.99},
		//		Alerts:            []*sloapi.Alert{}, // do nothing for now
		//	}
		//
		//	svcs := []*sloapi.Service{}
		//
		//	req := &sloapi.CreateSLORequest{
		//		SLO:      inputSLO,
		//		Services: svcs,
		//	}
		//	_, err := sloClient.CreateSLO(ctx, req)
		//	Expect(err).To(HaveOccurred())
		//	stat, ok := status.FromError(err)
		//	Expect(ok).To(BeTrue())
		//	Expect(stat.Code()).To(Equal(codes.InvalidArgument))
		//
		//	svcs = []*sloapi.Service{
		//		{
		//			JobId: "prometheus",
		//			// MetricName:    "http-availability",
		//			// MetricIdGood:  "prometheus_http_request_duration_seconds_count",
		//			// MetricIdTotal: "prometheus_http_request_duration_seconds_count",
		//			ClusterId: "agent",
		//		},
		//	}
		//	req.Services = svcs
		//	createdItems, err := sloClient.CreateSLO(ctx, req)
		//	Expect(err).To(Succeed())
		//	Expect(createdItems.Items).To(HaveLen(1))
		//	createdSlos = append(createdSlos, createdItems.Items...)
		//	// Need to check all three individual rules are created on the cortex backend
		//	expectSLOGroupToExist(adminClient, ctx, "agent", createdSlos[0].Id)
		//
		//})
		//XIt("Should list SLOs", func() {
		//	slos, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
		//	Expect(err).To(Succeed())
		//	Expect(slos.Items).To(HaveLen(len(createdSlos)))
		//})
		//
		//XIt("Should be able to get specific SLOs by Id", func() {
		//	Expect(createdSlos).NotTo(HaveLen(0))
		//	id := createdSlos[0].Id
		//	slo, err := sloClient.GetSLO(ctx, &corev1.Reference{Id: id})
		//	Expect(err).To(Succeed())
		//	Expect(slo.Service.ClusterId).To(Equal("agent"))
		//	Expect(slo.Service.JobId).To(Equal("prometheus"))
		//})
		//XIt("Should update valid SLOs", func() {
		//	Expect(createdSlos).NotTo(HaveLen(0))
		//	id := createdSlos[0].Id
		//	sloToUpdate, err := sloClient.GetSLO(ctx, &corev1.Reference{Id: id})
		//	Expect(err).To(Succeed())
		//	Expect(sloToUpdate.Service.ClusterId).ToNot(Equal("agent2"))
		//	Expect(sloToUpdate.SLO.Labels).ToNot(HaveLen(1))
		//	// change cluster of SLO
		//	newsvc := sloToUpdate.Service
		//	sloToUpdate.SLO.Labels = []*sloapi.Label{{Name: "adg"}}
		//	newsvc.ClusterId = "agent2"
		//	_, err = sloClient.UpdateSLO(ctx, &sloapi.SLOData{
		//		Id:      sloToUpdate.Id,
		//		SLO:     sloToUpdate.SLO,
		//		Service: newsvc,
		//	})
		//	Expect(err).To(Succeed())
		//
		//	updatedSLO, err := sloClient.GetSLO(ctx, &corev1.Reference{Id: id})
		//	Expect(err).To(Succeed())
		//	Expect(updatedSLO.Service.ClusterId).To(Equal("agent2"))
		//	// Check all three rules have been moved to the other cluster
		//	expectSLOGroupToExist(adminClient, ctx, "agent2", createdSlos[0].Id)
		//	// Check all three rules have been deleted from the original cluster
		//	expectSLOGroupNotToExist(adminClient, ctx, "agent", createdSlos[0].Id)
		//	Expect(updatedSLO.SLO.Labels).To(HaveLen(1))
		//
		//})
		//XIt("Should delete valid SLOs", func() {
		//	Expect(createdSlos).NotTo(HaveLen(0))
		//	id := createdSlos[0].Id
		//	_, err := sloClient.DeleteSLO(ctx, &corev1.Reference{Id: id})
		//	Expect(err).To(Succeed())
		//
		//	// Check all three rules have been delete from the cluster
		//	expectSLOGroupNotToExist(adminClient, ctx, "agent2", createdSlos[0].Id)
		//
		//	// For good measure, check this again, don't want any stragglers
		//	expectSLOGroupNotToExist(adminClient, ctx, "agent", createdSlos[0].Id)
		//
		//	createdSlos = createdSlos[1:]
		//
		//})
		//
		//XIt("Should clone SLOs", func() {
		//	inputSLO := &sloapi.ServiceLevelObjective{
		//		Name:              "test-slo",
		//		Datasource:        "monitoring",
		//		MetricName:        "http-availability",
		//		MonitorWindow:     "30d",                           // one of 30d, 28, 7d
		//		BudgetingInterval: durationpb.New(time.Minute * 5), // between 5m and 1h
		//		Labels:            []*sloapi.Label{},
		//		Target:            &sloapi.Target{Value: 99.99},
		//		Alerts:            []*sloapi.Alert{}, // do nothing for now
		//	}
		//	svcs := []*sloapi.Service{
		//		{
		//			JobId:     "prometheus",
		//			ClusterId: "agent",
		//		},
		//	}
		//
		//	req := &sloapi.CreateSLORequest{
		//		SLO:      inputSLO,
		//		Services: svcs,
		//	}
		//	createdItems, err := sloClient.CreateSLO(ctx, req)
		//	Expect(err).To(Succeed())
		//	Expect(createdItems.Items).To(HaveLen(1))
		//	for _, data := range createdItems.Items {
		//		createdSlos = append(createdSlos, data)
		//	}
		//	Expect(createdSlos).To(HaveLen(1))
		//
		//	_, err = sloClient.CloneSLO(ctx, &corev1.Reference{})
		//	Expect(err).To(HaveOccurred())
		//
		//	creationData, err := sloClient.CloneSLO(ctx, &corev1.Reference{Id: createdSlos[0].Id})
		//	Expect(err).To(Succeed())
		//	Expect(creationData.Id).NotTo(Equal(""))
		//	Expect(creationData.Id).NotTo(Equal(createdSlos[0].Id))
		//
		//	allSlos, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
		//	Expect(err).To(Succeed())
		//	Expect(allSlos.Items).To(HaveLen(len(createdSlos) + 1))
		//	createdSlos = append(createdSlos, &corev1.Reference{Id: creationData.Id})
		//
		//	expectRuleGroupToExist(adminClient, ctx, "agent", createdSlos[0].Id)
		//	expectRuleGroupToExist(adminClient, ctx, "agent", creationData.Id)
		//})
	})

	When("Reporting the Status of SLOs", func() {
		// XIt("should be able to reach the endpoints of the instrumentation server", func() {
		// 	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/uptime/good", instrumentationPort))
		// 	Expect(resp.StatusCode).To(Equal(200))
		// 	Expect(err).To(Succeed())
		// })
		//XIt("Should be able to get the status NoData of SLOs with no data", func() {
		//	Expect(createdSlos).To(HaveLen(2))
		//	refList, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
		//	Expect(err).To(Succeed())
		//	Expect(refList.Items).To(HaveLen(2))
		//
		//	status, err := sloClient.Status(ctx, &corev1.Reference{Id: createdSlos[0].Id})
		//	Expect(err).To(Succeed())
		//	// No HTTP requests are made agaisnt prometheus yet, so the status should be empty
		//	Expect(status.State).To(Equal(sloapi.SLOStatusState_NoData))
		//})
		//
		//XIt("Should be able to create an SLO for the instrumentation server for status testing", func() {
		//	inputSLO := &sloapi.ServiceLevelObjective{
		//		Name:              "test-slo",
		//		Datasource:        shared.MonitoringDatasource,
		//		MetricName:        instrumentationMetric,
		//		MonitorWindow:     "30d",                           // one of 30d, 28, 7d
		//		BudgetingInterval: durationpb.New(time.Minute * 5), // between 5m and 1h
		//		Labels:            []*sloapi.Label{},
		//		Target:            &sloapi.Target{Value: 99.99},
		//		Alerts:            []*sloapi.Alert{}, // do nothing for now
		//	}
		//	svcs := []*sloapi.Service{
		//		{
		//			JobId: query.MockTestServerName,
		//			// MetricName:    instrumentationMetric,
		//			// MetricIdGood:  "http_request_duration_seconds_count",
		//			// MetricIdTotal: "http_request_duration_seconds_count",
		//			ClusterId: "agent",
		//		},
		//	}
		//	req := &sloapi.CreateSLORequest{
		//		SLO:      inputSLO,
		//		Services: svcs,
		//	}
		//	idList, err := sloClient.CreateSLO(ctx, req)
		//	Expect(err).To(Succeed())
		//	Expect(idList.Items).To(HaveLen(1))
		//	instrumentationSLOID = &corev1.Reference{Id: idList.Items[0].Id}
		//})
		//
		//XIt("Should be able to get the status NoData of SLOs with no data", func() {
		//	port1, port2 := pPort, pPort2
		//	fmt.Println(port1, port2)
		//	Expect(canReachInstrumentationMetrics(instrumentationPort)).To(BeTrue())
		//	status, err := sloClient.Status(ctx, instrumentationSLOID)
		//	Expect(err).To(Succeed())
		//	// No HTTP requests are made agaisnt prometheus yet, so the status should be empty
		//	Expect(status.State).To(Equal(sloapi.SLOStatusState_NoData))
		//	numScrapes := 10
		//	for i := 0; i < numScrapes; i++ {
		//		goodE := simulateGoodStatus(instrumentationMetric, instrumentationPort, 1000)
		//		Expect(goodE).To(Equal(1000))
		//		time.Sleep(time.Second * 1)
		//	}
		//	simulateBadEvents(instrumentationMetric, instrumentationPort, 1000)
		//	time.Sleep(time.Second * 1)
		//	//time.Sleep(time.Minute)
		//	resp, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
		//		Tenants: []string{"agent"},
		//		Query:   fmt.Sprintf("http_request_duration_seconds_count{job=\"%s\"}", query.MockTestServerName),
		//	})
		//	Expect(err).NotTo(HaveOccurred())
		//	fmt.Println(resp.Data)
		//	respCodes, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
		//		Tenants: []string{"agent"},
		//		Query:   fmt.Sprintf("http_request_duration_seconds_count{job=\"%s\", code=~\"(2..|3..)\"}", query.MockTestServerName),
		//	})
		//	Expect(err).NotTo(HaveOccurred())
		//	fmt.Println(respCodes.Data)
		//
		//	resp2, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
		//		Tenants: []string{"agent"},
		//		Query:   fmt.Sprintf("rate(http_request_duration_seconds_count{job=\"%s\"}[10s])", query.MockTestServerName),
		//	})
		//	Expect(err).NotTo(HaveOccurred())
		//	fmt.Println(resp2.Data)
		//
		//	resp3, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
		//		Tenants: []string{"agent"},
		//		Query:   fmt.Sprintf("sum(rate(http_request_duration_seconds_count{job=\"%s\"}[5m]))", query.MockTestServerName),
		//	})
		//	Expect(err).NotTo(HaveOccurred())
		//	fmt.Println(resp3.Data)
		//
		//	resp4, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
		//		Tenants: []string{"agent"},
		//		Query:   "1 - (\n            (\n              (sum(rate(http_request_duration_seconds_count{job=\"MyServer\",code=~\"(2..|3..)\"}[5m])))\n            )\n            /\n            (\n              (sum(rate(http_request_duration_seconds_count{job=\"MyServer\"}[5m])))\n            )\n          )",
		//	})
		//	Expect(err).NotTo(HaveOccurred())
		//	fmt.Println(resp4.Data)
		//
		//	expectedGoodStatus, err := sloClient.Status(ctx, instrumentationSLOID)
		//	Expect(err).To(Succeed())
		//	Expect(expectedGoodStatus.State).To(Equal(sloapi.SLOStatusState_Ok))
		//})
	})

})
