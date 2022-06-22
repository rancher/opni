package slo_test

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	apis "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	slo1 := &apis.ServiceLevelObjective{
		Id:          "foo",
		Name:        "foo",
		Datasource:  "monitoring",
		Description: "Some SLO",
		Services: []*apis.Service{
			{JobId: "foo-service", ClusterId: "foo-cluster"},
		},
		MonitorWindow:     "30d",
		MetricDescription: "Some metric",
		BudgetingInterval: "5m",
		Labels:            []*apis.Label{},
		Targets: []*apis.Target{
			{ValueX100: 99},
		},
		Alerts: []*apis.Alert{},
	}
	var env *test.Environment
	var sloPlugin *slo.Plugin
	BeforeAll(func() {
		env = &test.Environment{
			TestBin: "../../../../testbin/bin",
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

		sloPlugin = slo.NewPlugin(ctx)
		sloPlugin.UseManagementAPI(client)
	})

	When("The SLO plugin starts", func() {
		It("should be able to discover services from downstream", func() {

			_, err := sloPlugin.ListServices(ctx, &emptypb.Empty{})
			Expect(err).To(MatchError(""))
		})
	})

	When("A ServiceLevelObjective message is given", func() {
		It("should validate proper input", func() {
			Expect(slo.ValidateInput(slo1)).To(Succeed())

			slo2 := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
			slo2.Datasource = "logging"
			Expect(slo.ValidateInput(slo2)).To(Succeed())
		})
		It("should reject improper input", func() {
			invalidDesc := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
			invalidDesc.Description = strings.Repeat("a", 1056)
			Expect(slo.ValidateInput(invalidDesc)).To(MatchError("Description must be between 1-1050 characters in length"))

			invalidSource := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
			invalidSource.Datasource = strings.Repeat("a", 256)
			Expect(slo.ValidateInput(invalidSource)).To(MatchError("Invalid required datasource value"))

			missingId := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
			missingId.Id = ""
			Expect(slo.ValidateInput(missingId)).To(MatchError("Internal error, unassigned SLO ID"))
		})
	})
	When("We convert a ServiceLevelObjective to OpenSLO", func() {
		It("Should create a valid OpenSLO spec", func() {
			Expect(slo1.Datasource).To(Equal("monitoring")) // make sure we didn't mutate original message
			_, err := slo.ParseToOpenSLO(slo1, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(Succeed())
		})
	})

	When("We convert an OpenSLO to Sloth IR", func() {
		It("should create a valid Sloth IR", func() {

		})
	})

	When("We convert Sloth IR to prometheus rules", func() {
		It("Should create valid prometheus rules", func() {

		})
	})

})
