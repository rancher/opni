package slo_test

import (
	"context"
	"fmt"
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
	sloTestDataDir := "../../../../pkg/test/testdata/slo/"
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

	multipleObjectives := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
	multiClusterMultiService := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)

	alertSLO := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
	alertSLO.Alerts = []*apis.Alert{ // multiple alerts
		//TODO : populate
	}

	multiAlerts := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
	multiAlerts.Alerts = []*apis.Alert{ // multiple alerts
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
			//Monitoring SLOs

			Expect(slo1.Datasource).To(Equal("monitoring")) // make sure we didn't mutate original message
			yamlSpecs, err := slo.ParseToOpenSLO(slo1, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(Succeed())
			Expect(yamlSpecs).To(HaveLen(1))

			expectedData, err := os.ReadFile(fmt.Sprintf("%s/slo1.yaml", sloTestDataDir))
			Expect(err).To(Succeed())

			createdData, err := yaml.Marshal(&yamlSpecs[0])
			Expect(createdData).To(MatchYAML(expectedData))
			multipleObjectives.Targets = []*apis.Target{
				{ValueX100: 9999},
				{ValueX100: 9995},
			}

			objectiveSpecs, err := slo.ParseToOpenSLO(multipleObjectives, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(Succeed())

			Expect(objectiveSpecs).To(HaveLen(1))
			expectedObjectives, err := os.ReadFile(fmt.Sprintf("%s/objectives.yaml", sloTestDataDir))
			Expect(yaml.Marshal(&objectiveSpecs[0])).To(MatchYAML(expectedObjectives))

			multiClusterMultiService.Services = []*apis.Service{
				{JobId: "foo-service", ClusterId: "foo-cluster"},
				{JobId: "foo-service2", ClusterId: "foo-cluster"},
				{JobId: "foo-service", ClusterId: "bar-cluster"},
				{JobId: "foo-service2", ClusterId: "bar-cluster"},
			}

			multiSpecs, err := slo.ParseToOpenSLO(multiClusterMultiService, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(Succeed())

			Expect(multiSpecs).To(HaveLen(4))
			expectedMulti, err := os.ReadFile(fmt.Sprintf("%s/multiSpecs.yaml", sloTestDataDir))
			Expect(err).To(Succeed())
			Expect(yaml.Marshal(multiSpecs)).To(MatchYAML(expectedMulti))

			// Logging SLOs
			sloLogging := proto.Clone(slo1).ProtoReflect().Interface().(*apis.ServiceLevelObjective)
			sloLogging.Datasource = "logging"
			_, err = slo.ParseToOpenSLO(sloLogging, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(MatchError("Not implemented"))
		})
	})

	When("We convert an OpenSLO to Sloth IR", func() {
		It("should create a valid Sloth IR", func() {
			//Monitoring SLOs
			yamlSpecs, err := slo.ParseToOpenSLO(slo1, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(Succeed())

			_, err = slo.ParseToPrometheusModel(yamlSpecs)
			Expect(err).To(Succeed())

		})
	})

	When("We convert Sloth IR to prometheus rules", func() {
		It("Should create valid prometheus rules", func() {

		})
	})

})
