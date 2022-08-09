package plugins_test

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/slo/query"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	apis "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	var sloClient apis.SLOClient
	var adminClient cortexadmin.CortexAdminClient
	// downstream server ports
	var pPort int
	var pPort2 int
	var instrumentationPort int
	var stopInstrumentationServer chan bool
	var instrumentationCancel context.CancelFunc

	var createdSlos []*corev1.Reference

	var instrumentationSLOID *corev1.Reference
	var instrumentationMetric string = "http-availability"
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
		fmt.Println("Starting server")
		instrumentationCtx, ca := context.WithCancel(context.Background())
		instrumentationCancel = ca
		instrumentationPort, stopInstrumentationServer = env.StartInstrumentationServer(instrumentationCtx)
		Expect(instrumentationPort).NotTo(Equal(0))
		Expect(canReachInstrumentationMetrics(instrumentationPort)).To(BeTrue())
		fmt.Println(instrumentationPort, stopInstrumentationServer)
		p, _ := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		pPort = env.StartPrometheus(p, test.NewOverridePrometheusConfig(
			"slo/prometheus/config.yaml",
			[]test.PrometheusJob{
				{
					JobName:    query.MockTestServerName,
					ScrapePort: instrumentationPort,
				},
			},
		))
		p2, _ := env.StartAgent("agent2", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		pPort2 = env.StartPrometheus(p2, test.NewOverridePrometheusConfig(
			"slo/prometheus/config.yaml",
			[]test.PrometheusJob{
				{
					JobName:    query.MockTestServerName,
					ScrapePort: instrumentationPort,
				},
			},
		))

		Expect(pPort != 0 && pPort2 != 0).To(BeTrue())
		sloClient = apis.NewSLOClient(env.ManagementClientConn())
		adminClient = cortexadmin.NewCortexAdminClient(env.ManagementClientConn())
		fmt.Println("Before all done")
		//time.Sleep(time.Second * 30)
	})

	When("The SLO plugin starts", func() {
		It("should be able to discover services from downstream", func() {
			sloSvcs, err := sloClient.ListServices(ctx, &apis.ListServiceRequest{
				Datasource: shared.MonitoringDatasource,
			})
			Expect(err).To(Succeed())
			Expect(sloSvcs.Items).To(HaveLen(4))

			expectedMap := map[string]bool{
				fmt.Sprintf("%s-%s", "agent", "prometheus"):              false,
				fmt.Sprintf("%s-%s", "agent", query.MockTestServerName):  false,
				fmt.Sprintf("%s-%s", "agent2", "prometheus"):             false,
				fmt.Sprintf("%s-%s", "agent2", query.MockTestServerName): false,
			}

			for _, sloSvc := range sloSvcs.Items {
				expectedMap[sloSvc.ClusterId+"-"+sloSvc.JobId] = true
			}

			foundUnique := 0
			for _, val := range expectedMap {
				if val {
					foundUnique++
				}
			}
			Expect(foundUnique).To(Equal(len(expectedMap)))
		})
	})

	When("Configuring what metrics are available", func() {
		It("should list available metrics", func() {
			metrics, err := sloClient.ListMetrics(ctx, &apis.ServiceList{})
			Expect(err).To(Succeed())
			Expect(metrics.Items).NotTo(HaveLen(0))
			for _, m := range metrics.Items {
				Expect(m.Description).NotTo(HaveLen(0))
			}
			keys := make([]string, 0, len(query.AvailableQueries))
			for k := range query.AvailableQueries {
				keys = append(keys, k)
			}
			for _, m := range metrics.Items {
				Expect(keys).To(ContainElement(m.Name))
			}
		})
		It("Should be able to assign pre-configured metrics to discrete metric ids", func() {
			_, err := sloClient.GetMetricId(ctx, &apis.MetricRequest{
				Name:       "http-availability",
				Datasource: shared.LoggingDatasource,
				ServiceId:  "prometheus",
				ClusterId:  "agent",
			})
			Expect(err).To(HaveOccurred())

			svc, err := sloClient.GetMetricId(ctx, &apis.MetricRequest{
				Name:       "uptime",
				Datasource: shared.MonitoringDatasource,
				ServiceId:  "prometheus",
				ClusterId:  "agent",
			})
			Expect(err).To(Succeed())
			Expect(svc.MetricName).To(Equal("uptime"))
			Expect(svc.MetricIdGood).To(Equal("up"))
			Expect(svc.MetricIdTotal).To(Equal("up"))

			latency, err := sloClient.GetMetricId(ctx, &apis.MetricRequest{
				Name:       "http-latency",
				Datasource: shared.MonitoringDatasource,
				ServiceId:  "prometheus",
				ClusterId:  "agent",
			})
			Expect(err).To(Succeed())

			Expect(latency.MetricIdGood).To(Equal("prometheus_http_request_duration_seconds_bucket"))
			Expect(latency.MetricIdTotal).To(Equal("prometheus_http_request_duration_seconds_count"))

			_, err = sloClient.GetMetricId(ctx, &apis.MetricRequest{
				Name:       "does not exist",
				Datasource: shared.MonitoringDatasource,
				ServiceId:  "prometheus",
				ClusterId:  "agent",
			})
			Expect(err).To(HaveOccurred())
		})

		It(
			`Should be able to list all preconfigured metrics available 
			to a given set of selected services Services`, func() {
				svcs, err := sloClient.ListServices(ctx, &apis.ListServiceRequest{
					Datasource: shared.MonitoringDatasource,
				})
				Expect(err).To(Succeed())
				Expect(svcs.Items).To(HaveLen(4))
				// match to prometheus & instrumentation server on both downstream clusters
				availableMetrics, err := sloClient.ListMetrics(ctx, svcs)
				Expect(err).ToNot(HaveOccurred())
				Expect(availableMetrics.Items).To(HaveLen(3))
			})
	})

	When("CRUDing SLOs", func() {
		It("Should create valid SLOs", func() {
			inputSLO := &apis.ServiceLevelObjective{
				Name:              "test-slo",
				Datasource:        shared.MonitoringDatasource,
				MetricName:        "http-availability",
				MonitorWindow:     "30d",                           // one of 30d, 28, 7d
				BudgetingInterval: durationpb.New(time.Minute * 5), // between 5m and 1h
				Labels:            []*apis.Label{{Name: "env"}, {Name: "dev"}},
				Target:            &apis.Target{Value: 99.99},
				Alerts:            []*apis.Alert{}, // do nothing for now
			}

			svcs := []*apis.Service{}

			req := &apis.CreateSLORequest{
				SLO:      inputSLO,
				Services: svcs,
			}
			_, err := sloClient.CreateSLO(ctx, req)
			Expect(err).To(HaveOccurred())
			stat, ok := status.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(stat.Code()).To(Equal(codes.InvalidArgument))

			svcs = []*apis.Service{
				{
					JobId: "prometheus",
					// MetricName:    "http-availability",
					// MetricIdGood:  "prometheus_http_request_duration_seconds_count",
					// MetricIdTotal: "prometheus_http_request_duration_seconds_count",
					ClusterId: "agent",
				},
			}
			req.Services = svcs
			createdItems, err := sloClient.CreateSLO(ctx, req)
			Expect(err).To(Succeed())
			Expect(createdItems.Items).To(HaveLen(1))
			createdSlos = append(createdSlos, createdItems.Items...)
			// Need to check all three individual rules are created on the cortex backend
			expectSLOGroupToExist(adminClient, ctx, "agent", createdSlos[0].Id)

		})
		It("Should list SLOs", func() {
			slos, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).To(Succeed())
			Expect(slos.Items).To(HaveLen(len(createdSlos)))
		})

		It("Should be able to get specific SLOs by Id", func() {
			Expect(createdSlos).NotTo(HaveLen(0))
			id := createdSlos[0].Id
			slo, err := sloClient.GetSLO(ctx, &corev1.Reference{Id: id})
			Expect(err).To(Succeed())
			Expect(slo.Service.ClusterId).To(Equal("agent"))
			Expect(slo.Service.JobId).To(Equal("prometheus"))
		})
		It("Should update valid SLOs", func() {
			Expect(createdSlos).NotTo(HaveLen(0))
			id := createdSlos[0].Id
			sloToUpdate, err := sloClient.GetSLO(ctx, &corev1.Reference{Id: id})
			Expect(err).To(Succeed())
			Expect(sloToUpdate.Service.ClusterId).ToNot(Equal("agent2"))
			Expect(sloToUpdate.SLO.Labels).ToNot(HaveLen(1))
			// change cluster of SLO
			newsvc := sloToUpdate.Service
			sloToUpdate.SLO.Labels = []*apis.Label{{Name: "adg"}}
			newsvc.ClusterId = "agent2"
			_, err = sloClient.UpdateSLO(ctx, &apis.SLOData{
				Id:      sloToUpdate.Id,
				SLO:     sloToUpdate.SLO,
				Service: newsvc,
			})
			Expect(err).To(Succeed())

			updatedSLO, err := sloClient.GetSLO(ctx, &corev1.Reference{Id: id})
			Expect(err).To(Succeed())
			Expect(updatedSLO.Service.ClusterId).To(Equal("agent2"))
			// Check all three rules have been moved to the other cluster
			expectSLOGroupToExist(adminClient, ctx, "agent2", createdSlos[0].Id)
			// Check all three rules have been deleted from the original cluster
			expectSLOGroupNotToExist(adminClient, ctx, "agent", createdSlos[0].Id)
			Expect(updatedSLO.SLO.Labels).To(HaveLen(1))

		})
		It("Should delete valid SLOs", func() {
			Expect(createdSlos).NotTo(HaveLen(0))
			id := createdSlos[0].Id
			_, err := sloClient.DeleteSLO(ctx, &corev1.Reference{Id: id})
			Expect(err).To(Succeed())

			// Check all three rules have been delete from the cluster
			expectSLOGroupNotToExist(adminClient, ctx, "agent2", createdSlos[0].Id)

			// For good measure, check this again, don't want any stragglers
			expectSLOGroupNotToExist(adminClient, ctx, "agent", createdSlos[0].Id)

			createdSlos = createdSlos[1:]

		})

		It("Should clone SLOs", func() {
			inputSLO := &apis.ServiceLevelObjective{
				Name:              "test-slo",
				Datasource:        "monitoring",
				MetricName:        "http-availability",
				MonitorWindow:     "30d",                           // one of 30d, 28, 7d
				BudgetingInterval: durationpb.New(time.Minute * 5), // between 5m and 1h
				Labels:            []*apis.Label{},
				Target:            &apis.Target{Value: 99.99},
				Alerts:            []*apis.Alert{}, // do nothing for now
			}
			svcs := []*apis.Service{
				{
					JobId:     "prometheus",
					ClusterId: "agent",
				},
			}

			req := &apis.CreateSLORequest{
				SLO:      inputSLO,
				Services: svcs,
			}
			createdItems, err := sloClient.CreateSLO(ctx, req)
			Expect(err).To(Succeed())
			Expect(createdItems.Items).To(HaveLen(1))
			for _, data := range createdItems.Items {
				createdSlos = append(createdSlos, data)
			}
			Expect(createdSlos).To(HaveLen(1))

			_, err = sloClient.CloneSLO(ctx, &corev1.Reference{})
			Expect(err).To(HaveOccurred())

			creationData, err := sloClient.CloneSLO(ctx, &corev1.Reference{Id: createdSlos[0].Id})
			Expect(err).To(Succeed())
			Expect(creationData.Id).NotTo(Equal(""))
			Expect(creationData.Id).NotTo(Equal(createdSlos[0].Id))

			allSlos, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).To(Succeed())
			Expect(allSlos.Items).To(HaveLen(len(createdSlos) + 1))
			createdSlos = append(createdSlos, &corev1.Reference{Id: creationData.Id})

			expectRuleGroupToExist(adminClient, ctx, "agent", createdSlos[0].Id)
			expectRuleGroupToExist(adminClient, ctx, "agent", creationData.Id)
		})
	})

	When("Reporting the Status of SLOs", func() {
		// It("should be able to reach the endpoints of the instrumentation server", func() {
		// 	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/uptime/good", instrumentationPort))
		// 	Expect(resp.StatusCode).To(Equal(200))
		// 	Expect(err).To(Succeed())
		// })
		It("Should be able to get the status NoData of SLOs with no data", func() {
			Expect(createdSlos).To(HaveLen(2))
			refList, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).To(Succeed())
			Expect(refList.Items).To(HaveLen(2))

			status, err := sloClient.Status(ctx, &corev1.Reference{Id: createdSlos[0].Id})
			Expect(err).To(Succeed())
			// No HTTP requests are made agaisnt prometheus yet, so the status should be empty
			Expect(status.State).To(Equal(apis.SLOStatusState_NoData))
		})

		It("Should be able to create an SLO for the instrumentation server for status testing", func() {
			inputSLO := &apis.ServiceLevelObjective{
				Name:              "test-slo",
				Datasource:        shared.MonitoringDatasource,
				MetricName:        instrumentationMetric,
				MonitorWindow:     "30d",                           // one of 30d, 28, 7d
				BudgetingInterval: durationpb.New(time.Minute * 5), // between 5m and 1h
				Labels:            []*apis.Label{},
				Target:            &apis.Target{Value: 99.99},
				Alerts:            []*apis.Alert{}, // do nothing for now
			}
			svcs := []*apis.Service{
				{
					JobId: query.MockTestServerName,
					// MetricName:    instrumentationMetric,
					// MetricIdGood:  "http_request_duration_seconds_count",
					// MetricIdTotal: "http_request_duration_seconds_count",
					ClusterId: "agent",
				},
			}
			req := &apis.CreateSLORequest{
				SLO:      inputSLO,
				Services: svcs,
			}
			idList, err := sloClient.CreateSLO(ctx, req)
			Expect(err).To(Succeed())
			Expect(idList.Items).To(HaveLen(1))
			instrumentationSLOID = &corev1.Reference{Id: idList.Items[0].Id}
		})

		It("Should be able to get the status NoData of SLOs with no data", func() {
			port1, port2 := pPort, pPort2
			fmt.Println(port1, port2)
			Expect(canReachInstrumentationMetrics(instrumentationPort)).To(BeTrue())
			status, err := sloClient.Status(ctx, instrumentationSLOID)
			Expect(err).To(Succeed())
			// No HTTP requests are made agaisnt prometheus yet, so the status should be empty
			Expect(status.State).To(Equal(apis.SLOStatusState_NoData))

			goodE := simulateGoodStatus(instrumentationMetric, instrumentationPort, 1000)
			Expect(goodE).To(Equal(1000))
			//time.Sleep(time.Minute)
			resp, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
				Tenants: []string{"agent"},
				Query:   fmt.Sprintf("http_request_duration_seconds_count{job=\"%s\"}", query.MockTestServerName),
			})
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(resp.Data)

			resp2, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
				Tenants: []string{"agent2"},
				Query:   fmt.Sprintf("http_request_duration_seconds_count{job=\"%s\"}", query.MockTestServerName),
			})
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(resp2.Data)

			expectedGoodStatus, err := sloClient.Status(ctx, instrumentationSLOID)
			Expect(err).To(Succeed())
			Expect(expectedGoodStatus.State).To(Equal(apis.SLOStatusState_Ok))
			// stopInstrumentationServer <- true
			instrumentationCancel()
		})
	})

})
