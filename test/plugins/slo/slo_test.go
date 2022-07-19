package plugins_test

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sync"
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
	"github.com/rancher/opni/plugins/slo/pkg/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func sloCortexGroupsToCheck(groupName string) []string {
	return []string{
		groupName + slo.RecordingRuleSuffix,
		groupName + slo.MetadataRuleSuffix,
		groupName + slo.AlertRuleSuffix,
	}
}

func expectSLOGroupToExist(adminClient cortexadmin.CortexAdminClient, ctx context.Context, tenant string, groupName string) {
	var anyError error
	var wg sync.WaitGroup
	groupsToCheck := sloCortexGroupsToCheck(groupName)
	wg.Add(len(groupsToCheck))

	for _, group := range groupsToCheck {
		groupToCheck := group
		go func() {
			defer wg.Done()
			if err := expectRuleGroupToExist(adminClient, ctx, tenant, groupToCheck); err != nil {
				anyError = err
			}
		}()
	}
	wg.Wait()
	Expect(anyError).Should(BeNil())
}

func expectSLOGroupNotToExist(adminClient cortexadmin.CortexAdminClient, ctx context.Context, tenant string, groupName string) {
	var anyError error
	var wg sync.WaitGroup
	groupsToCheck := sloCortexGroupsToCheck(groupName)
	wg.Add(len(groupsToCheck))

	for _, group := range groupsToCheck {
		groupToCheck := group
		go func() {
			defer wg.Done()
			if err := expectRuleGroupNotToExist(adminClient, ctx, tenant, groupToCheck); err != nil {
				anyError = err
			}
		}()
	}
	wg.Wait()
	Expect(anyError).Should(BeNil())
}

// potentially "long" running function, call asynchronously
func expectRuleGroupToExist(adminClient cortexadmin.CortexAdminClient, ctx context.Context, tenant string, groupName string) error {
	for i := 0; i < 10; i++ {
		resp, err := adminClient.GetRule(ctx, &cortexadmin.RuleRequest{
			Tenant:    tenant,
			GroupName: groupName,
		})
		if err == nil {
			Expect(resp.Data).To(Not(BeNil()))
			return nil
		}
		time.Sleep(1)
	}
	return fmt.Errorf("Rule %s should exist, but doesn't", groupName)
}

// potentially "long" running function, call asynchronously
func expectRuleGroupNotToExist(adminClient cortexadmin.CortexAdminClient, ctx context.Context, tenant string, groupName string) error {
	for i := 0; i < 10; i++ {
		_, err := adminClient.GetRule(ctx, &cortexadmin.RuleRequest{
			Tenant:    tenant,
			GroupName: groupName,
		})
		if err != nil {
			Expect(status.Code(err)).To(Equal(codes.NotFound))
			return nil
		}

		time.Sleep(1)
	}
	return fmt.Errorf("Rule %s still exists, but shouldn't", groupName)
}

func simulateGoodEvents(metricName string, instrumentationServerPort int, numEvents int) {
	for i := 0; i < numEvents; i++ {
		go func() {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/%s/good", instrumentationServerPort, metricName))
			Expect(resp.StatusCode).To(Equal(200))
			Expect(err).To(Succeed())
		}()
	}
}

func simulateBadEvents(metricName string, instrumentationServerPort int, numEvents int) {

	for i := 0; i < numEvents; i++ {
		go func() {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/%s/bad", instrumentationServerPort, metricName))
			Expect(resp.StatusCode).To(Equal(200))
			Expect(err).To(Succeed())
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
	var mockServerName string = "mock-server"

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
		instrumentationPort, stopInstrumentationServer = env.StartInstrumentationServer()
		fmt.Println(instrumentationPort, stopInstrumentationServer)
		p, _ := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		pPort = env.StartPrometheus(p, test.NewAdditionalPrometheusConfig(
			"slo/prometheus/config.yaml",
			[]test.PrometheusJob{
				{
					JobName:    mockServerName,
					ScrapePort: instrumentationPort,
				},
			},
		))
		p2, _ := env.StartAgent("agent2", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		pPort2 = env.StartPrometheus(p2, test.NewAdditionalPrometheusConfig(
			"slo/prometheus/config.yaml",
			[]test.PrometheusJob{
				{
					JobName:    mockServerName,
					ScrapePort: instrumentationPort,
				},
			},
		))

		Expect(pPort != 0 && pPort2 != 0).To(BeTrue())
		sloClient = apis.NewSLOClient(env.ManagementClientConn())
		adminClient = cortexadmin.NewCortexAdminClient(env.ManagementClientConn())
		fmt.Println("Before all done")
	})

	When("The SLO plugin starts", func() {
		It("should be able to discover services from downstream", func() {
			time.Sleep(10 * time.Second)
			sloSvcs, err := sloClient.ListServices(ctx, &emptypb.Empty{})
			Expect(err).To(Succeed())
			Expect(sloSvcs.Items).To(HaveLen(4))

			expectedMap := map[string]bool{
				fmt.Sprintf("%s-%s", "agent", "prometheus"):    false,
				fmt.Sprintf("%s-%s", "agent", mockServerName):  false,
				fmt.Sprintf("%s-%s", "agent2", "prometheus"):   false,
				fmt.Sprintf("%s-%s", "agent2", mockServerName): false,
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
			metrics, err := sloClient.ListMetrics(ctx, &emptypb.Empty{})
			Expect(err).To(Succeed())
			Expect(metrics.Items).NotTo(HaveLen(0))
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
	})

	When("CRUDing SLOs", func() {
		It("Should create valid SLOs", func() {
			inputSLO := &apis.ServiceLevelObjective{
				Name:              "test-slo",
				Datasource:        shared.MonitoringDatasource,
				MonitorWindow:     "30d",                           // one of 30d, 28, 7d
				BudgetingInterval: durationpb.New(time.Minute * 5), // between 5m and 1h
				Labels:            []*apis.Label{},
				Target:            &apis.Target{ValueX100: 9999},
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
					JobId:         "prometheus",
					MetricName:    "http-availability",
					MetricIdGood:  "prometheus_http_request_duration_seconds_count",
					MetricIdTotal: "prometheus_http_request_duration_seconds_count",
					ClusterId:     "agent",
				},
			}
			req.Services = svcs
			createdItems, err := sloClient.CreateSLO(ctx, req)
			Expect(err).To(Succeed())
			Expect(createdItems.Items).To(HaveLen(1))
			for _, item := range createdItems.Items {
				createdSlos = append(createdSlos, item)
			}
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
			// change cluster of SLO
			newsvc := sloToUpdate.Service
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
				MonitorWindow:     "30d",                           // one of 30d, 28, 7d
				BudgetingInterval: durationpb.New(time.Minute * 5), // between 5m and 1h
				Labels:            []*apis.Label{},
				Target:            &apis.Target{ValueX100: 9999},
				Alerts:            []*apis.Alert{}, // do nothing for now
			}
			svcs := []*apis.Service{
				{
					JobId:         "prometheus",
					MetricName:    "http-availability",
					MetricIdGood:  "http_request_duration_seconds_count",
					MetricIdTotal: "http_request_duration_seconds_count",
					ClusterId:     "agent",
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
				MonitorWindow:     "30d",                           // one of 30d, 28, 7d
				BudgetingInterval: durationpb.New(time.Minute * 5), // between 5m and 1h
				Labels:            []*apis.Label{},
				Target:            &apis.Target{ValueX100: 9999},
				Alerts:            []*apis.Alert{}, // do nothing for now
			}
			svcs := []*apis.Service{
				{
					JobId:         mockServerName,
					MetricName:    instrumentationMetric,
					MetricIdGood:  "http_request_duration_seconds_count",
					MetricIdTotal: "http_request_duration_seconds_count",
					ClusterId:     "agent",
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
			status, err := sloClient.Status(ctx, instrumentationSLOID)
			Expect(err).To(Succeed())
			// No HTTP requests are made agaisnt prometheus yet, so the status should be empty
			Expect(status.State).To(Equal(apis.SLOStatusState_NoData))

			goodE := simulateGoodStatus(instrumentationMetric, instrumentationPort, 10)
			Expect(goodE).To(Equal(10))
			status, err = sloClient.Status(ctx, instrumentationSLOID)
			Expect(err).To(Succeed())
			// Expect(status.State).To(Equal(apis.SLOStatusState_Ok))
			// stopInstrumentationServer <- true
		})
	})

})
