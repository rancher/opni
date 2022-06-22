package slo_test

import (
	"context"
	"os"
	"strings"

	"github.com/hashicorp/go-hclog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	apis "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v2"
)

var _ = Describe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label(test.Unit, test.Slow), func() {

	slo1 := &apis.ServiceLevelObjective{ // no alerts
		Id:          "foo-id",
		Name:        "foo-name",
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
			{ValueX100: 9999},
		},
		Alerts: []*apis.Alert{},
	}

	slo2 := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
	slo2.Alerts = []*apis.Alert{ // multiple alerts
		//TODO : populate
	}

	When("A ServiceLevelObjective message is given", func() {
		It("should validate proper input", func() {
			Expect(slo.ValidateInput(slo1)).To(Succeed())

			sloLogging := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
			sloLogging.Datasource = "logging"
			Expect(slo.ValidateInput(sloLogging)).To(Succeed())
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
			yamlSpec, err := slo.ParseToOpenSLO(slo1, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(Succeed())

			data, err := yaml.Marshal(yamlSpec)
			Expect(err).To(Succeed())

			expectedData, err := os.ReadFile("../../../../pkg/test/testdata/slo/slo1.yaml")

			Expect(err).To(Succeed())
			Expect(data).ToNot(Equal(expectedData))

			sloLogging := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
			sloLogging.Datasource = "logging"
			_, err = slo.ParseToOpenSLO(sloLogging, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(MatchError("Not implemented"))
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
