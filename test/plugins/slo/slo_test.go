package plugins_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	apis "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	var env *test.Environment
	var sloClient apis.SLOClient
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
		env.StartPrometheus(p)
		p2, _ := env.StartAgent("agent2", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		env.StartPrometheus(p2)
		sloClient = apis.NewSLOClient(env.ManagementClientConn())
	})

	When("The SLO plugin starts", func() {
		It("should be able to discover services from downstream", func() {

			sloSvcs, err := sloClient.ListServices(ctx, &emptypb.Empty{})
			Expect(err).To(Succeed())
			Expect(sloSvcs.Items).To(HaveLen(2))

			Expect(sloSvcs.Items[0].ClusterId).To(Equal("agent"))
			Expect(sloSvcs.Items[0].JobId).To(Equal("prometheus"))
			Expect(sloSvcs.Items[1].ClusterId).To(Equal("agent2"))
			Expect(sloSvcs.Items[1].JobId).To(Equal("prometheus"))
		})

		It("should be able to fetch distinct services", func() {
			_, err := sloClient.GetService(ctx, &corev1.Reference{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("Configuring what metrics are available", func() {
		It("should list available metrics", func() {
			_, err := sloClient.ListMetrics(ctx, &emptypb.Empty{})
			Expect(err).To(HaveOccurred())
		})
		It("Should be able to fetch distinct metrics", func() {
			_, err := sloClient.GetMetric(ctx, &apis.MetricRequest{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("CRUDing SLOs", func() {
		It("Should create valid SLOs", func() {
			input := &apis.ServiceLevelObjective{
				Id:          "", // will be generated
				Name:        "test-slo",
				Datasource:  "monitoring",
				Description: "test slo",
				Services: []*apis.Service{
					{
						JobId:      "prometheus",
						ClusterId:  "agent",
						MetricName: "uptime",
						MetricId:   "up",
					},
				},
				MonitorWindow:     "5m",
				MetricDescription: "test metric",
				BudgetingInterval: "30d",
				Labels:            []*apis.Label{},
				Targets: []*apis.Target{
					{
						ValueX100: 9999,
					},
				},
				Alerts: []*apis.Alert{},
			}
			_, err := sloClient.CreateSLO(ctx, input)
			Expect(err).To(Succeed())
		})
		It("Should update valid SLOs", func() {
			_, err := sloClient.UpdateSLO(ctx, &apis.ServiceLevelObjective{})
			Expect(err).To(HaveOccurred())
		})
		It("Should delete valid SLOs", func() {
			_, err := sloClient.DeleteSLO(ctx, &corev1.Reference{})
			Expect(err).To(HaveOccurred())
		})
		It("Should list SLOs", func() {
			_, err := sloClient.ListSLOs(ctx, &emptypb.Empty{})
			Expect(err).To(HaveOccurred())
		})

		It("Should clone SLOs", func() {
			_, err := sloClient.CloneSLO(ctx, &corev1.Reference{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("Reporting the State of SLOs", func() {
		It("Should report the state of SLOs", func() {
			_, err := sloClient.GetState(ctx, &corev1.Reference{})
			Expect(err).To(HaveOccurred())
		})

		It("Should be able to set the state manually", func() {
			_, err := sloClient.SetState(ctx, &apis.SetStateRequest{})
			Expect(err).To(HaveOccurred())
		})
	})

})
