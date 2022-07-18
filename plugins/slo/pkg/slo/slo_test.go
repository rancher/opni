package slo_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	oslov1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/alexandreLamarre/sloth/core/alert"
	"github.com/alexandreLamarre/sloth/core/prometheus"
	"github.com/hashicorp/go-hclog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/test"
	apis "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"gopkg.in/yaml.v2"
)

var _ = Describe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label(test.Unit, test.Slow), func() {
	sloTestDataDir := "../../../../pkg/test/testdata/slo/"
	slo1Objective := &apis.ServiceLevelObjective{ // no alerts
		Name:              "foo-name",
		Datasource:        "monitoring",
		MonitorWindow:     "30d",
		BudgetingInterval: durationpb.New(time.Minute * 5),
		Labels:            []*apis.Label{},
		Target: &apis.Target{
			ValueX100: 9999,
		},
		Alerts: []*apis.Alert{},
	}

	svcs := []*apis.Service{
		{JobId: "foo-service", ClusterId: "foo-cluster", MetricName: "uptime", MetricIdGood: "up", MetricIdTotal: "up"},
	}
	slo1 := &apis.CreateSLORequest{
		SLO:      slo1Objective,
		Services: svcs,
	}

	multiClusterMultiService := proto.Clone(slo1).ProtoReflect().Interface().(*apis.CreateSLORequest)

	alertSLO := proto.Clone(slo1).ProtoReflect().Interface().(*apis.CreateSLORequest)
	alertSLO.SLO.Alerts = []*apis.Alert{ // multiple alerts
		{
			Name:                    "alert-foo",
			NotificationTarget:      "email",
			NotificationDescription: "Send to email",
			Description:             "Alert when we breach the objective",
			ConditionType:           "budget",
			ThresholdType:           ">",
			OnNoData:                true,
			OnCreate:                true,
			OnBreach:                true,
			OnResolved:              true,
		},
	}

	multiAlerts := proto.Clone(slo1).ProtoReflect().Interface().(*apis.CreateSLORequest)
	multiAlerts.SLO.Alerts = alertSLO.SLO.Alerts
	multiAlerts.SLO.Alerts = append(multiAlerts.SLO.Alerts, &apis.Alert{
		Name:                    "alert-bar",
		NotificationTarget:      "slack",
		NotificationDescription: "Send to slack",
		Description:             "Alert on burn rate",
		ConditionType:           "burnrate",
		ThresholdType:           ">",
		OnNoData:                true,
		OnCreate:                true,
		OnBreach:                true,
		OnResolved:              true,
	})

	var simpleSpec []oslov1.SLO
	var multiClusterSpecs []oslov1.SLO
	var alertSpecs []oslov1.SLO
	var multiAlertSpecs []oslov1.SLO

	var simplePrometheusIR []*prometheus.SLOGroup
	var objectivePrometheusIR []*prometheus.SLOGroup
	var multiClusterPrometheusIR []*prometheus.SLOGroup
	// var alertPrometheusIR []*prometheus.SLOGroup
	// var multiAlertPrometheusIR []*prometheus.SLOGroup

	var simplePrometheusResponse []slo.SLORuleFmtWrapper
	var objectivePrometheusResponse []slo.SLORuleFmtWrapper
	var multiClusterPrometheusResponse []slo.SLORuleFmtWrapper
	// var alertPrometheusIR []slo.SLORuleFmtWrapper
	// var multiAlertPrometheusIR []slo.SLORuleFmtWrapper

	When("A ServiceLevelObjective message is given", func() {
		It("should validate proper input", func() {
			Expect(slo.ValidateInput(slo1)).To(Succeed())

			sloLogging := proto.Clone(slo1).ProtoReflect().Interface().(*apis.CreateSLORequest)
			sloLogging.SLO.Datasource = "logging"
			Expect(slo.ValidateInput(sloLogging)).To(Succeed())

			Expect(slo.ValidateInput(alertSLO)).To(Succeed())
			Expect(slo.ValidateInput(multiAlerts)).To(Succeed())

			for _, atype := range []string{shared.NotifHook, shared.NotifPager, shared.NotifMail, shared.NotifSlack} {
				sloNewAlert := proto.Clone(alertSLO).ProtoReflect().Interface().(*apis.CreateSLORequest)
				alertSLO.SLO.Alerts[0].NotificationTarget = atype
				Expect(slo.ValidateInput(sloNewAlert)).To(Succeed())
			}

			for _, ctype := range []string{shared.AlertingBurnRate, shared.AlertingBudget, shared.AlertingTarget} {
				sloNewAlert := proto.Clone(alertSLO).ProtoReflect().Interface().(*apis.CreateSLORequest)
				alertSLO.SLO.Alerts[0].ConditionType = ctype
				Expect(slo.ValidateInput(sloNewAlert)).To(Succeed())
			}

			for _, ttype := range []string{shared.GTThresholdType, shared.LTThresholdType} {
				sloNewAlert := proto.Clone(alertSLO).ProtoReflect().Interface().(*apis.CreateSLORequest)
				alertSLO.SLO.Alerts[0].ThresholdType = ttype
				Expect(slo.ValidateInput(sloNewAlert)).To(Succeed())
			}

		})
		It("should reject improper input", func() {

			invalidSource := proto.Clone(slo1).ProtoReflect().Interface().(*apis.CreateSLORequest)
			invalidSource.SLO.Datasource = strings.Repeat("a", 256)
			Expect(slo.ValidateInput(invalidSource)).To(MatchError(shared.ErrInvalidDatasource))

			sloInvalidAlertTarget := proto.Clone(alertSLO).ProtoReflect().Interface().(*apis.CreateSLORequest)
			sloInvalidAlertTarget.SLO.Alerts[0].NotificationTarget = "invalid-234987ukjas"
			Expect(slo.ValidateInput(sloInvalidAlertTarget)).To(MatchError(shared.ErrInvalidAlertTarget))

			sloInvalidAlertCondition := proto.Clone(alertSLO).ProtoReflect().Interface().(*apis.CreateSLORequest)
			sloInvalidAlertCondition.SLO.Alerts[0].ConditionType = "invalid-234987ukjas"
			Expect(slo.ValidateInput(sloInvalidAlertCondition)).To(MatchError(shared.ErrInvalidAlertCondition))

			sloInvalidAlertThreshold := proto.Clone(alertSLO).ProtoReflect().Interface().(*apis.CreateSLORequest)
			sloInvalidAlertThreshold.SLO.Alerts[0].ThresholdType = "invalid-234987ukjas"
			Expect(slo.ValidateInput(sloInvalidAlertThreshold)).To(MatchError(shared.ErrInvalidAlertThreshold))

		})
	})
	When("We convert a ServiceLevelObjective to OpenSLO", func() {
		It("Should create a valid OpenSLO spec", func() {
			//Monitoring SLOs
			var err error
			Expect(slo1.SLO.Datasource).To(Equal("monitoring")) // make sure we didn't mutate original message
			simpleSpec, err = slo.ParseToOpenSLO(slo1, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(Succeed())
			Expect(simpleSpec).To(HaveLen(1))

			expectedData, err := os.ReadFile(fmt.Sprintf("%s/slo1.yaml", sloTestDataDir))
			Expect(err).To(Succeed())

			createdData, err := yaml.Marshal(&simpleSpec[0])
			Expect(createdData).To(MatchYAML(expectedData))

			multiClusterMultiService.Services = []*apis.Service{
				{JobId: "foo-service", ClusterId: "foo-cluster", MetricName: "uptime", MetricIdGood: "up", MetricIdTotal: "up"},
				{JobId: "foo-service2", ClusterId: "foo-cluster", MetricName: "uptime", MetricIdGood: "up", MetricIdTotal: "up"},
				{JobId: "foo-service", ClusterId: "bar-cluster", MetricName: "uptime", MetricIdGood: "up", MetricIdTotal: "up"},
				{JobId: "foo-service2", ClusterId: "bar-cluster", MetricName: "uptime", MetricIdGood: "up", MetricIdTotal: "up"},
			}

			multiClusterSpecs, err = slo.ParseToOpenSLO(multiClusterMultiService, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(Succeed())

			Expect(multiClusterSpecs).To(HaveLen(4))
			expectedMulti, err := os.ReadFile(fmt.Sprintf("%s/multiSpecs.yaml", sloTestDataDir))
			Expect(err).To(Succeed())
			Expect(yaml.Marshal(multiClusterSpecs)).To(MatchYAML(expectedMulti))

			// Alerting SLOS
			alertSpecs, err = slo.ParseToOpenSLO(alertSLO, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(Succeed())
			Expect(alertSpecs).To(HaveLen(1))
			Expect(alertSpecs[0].Spec.AlertPolicies).To(HaveLen(1))
			//FIXME: this is a check to verify we haven't implemented this functionality yet
			Expect(alertSpecs[0].Spec.AlertPolicies[0].Spec.Conditions).To(HaveLen(0))
			expectedAlert, err := os.ReadFile(fmt.Sprintf("%s/alertSLO.yaml", sloTestDataDir))
			Expect(err).To(Succeed())
			Expect(yaml.Marshal(alertSpecs[0])).To(MatchYAML(expectedAlert))

			multiAlertSpecs, err = slo.ParseToOpenSLO(multiAlerts, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(Succeed())
			expectedMultiAlert, err := os.ReadFile(fmt.Sprintf("%s/multiAlertSLO.yaml", sloTestDataDir))
			Expect(err).To(Succeed())
			Expect(yaml.Marshal(multiAlertSpecs)).To(MatchYAML(expectedMultiAlert))

			// Logging SLOs
			sloLogging := proto.Clone(slo1).ProtoReflect().Interface().(*apis.CreateSLORequest)
			sloLogging.SLO.Datasource = "logging"
			_, err = slo.ParseToOpenSLO(sloLogging, context.Background(), hclog.New(&hclog.LoggerOptions{}))
			Expect(err).To(HaveOccurred())
			stat, ok := status.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(stat.Code()).To(Equal(codes.Unimplemented))
		})
	})

	When("We convert an OpenSLO to Sloth IR for OpniMonitoring", func() {
		It("should create a valid Sloth IR", func() {
			for _, ss := range simpleSpec {
				a, err := slo.ParseToPrometheusModel(ss)
				Expect(err).To(Succeed())
				simplePrometheusIR = append(simplePrometheusIR, a)
			}
			Expect(simplePrometheusIR).To(HaveLen(1))

			for _, ms := range multiClusterSpecs {
				a, err := slo.ParseToPrometheusModel(ms)
				Expect(err).To(Succeed())
				multiClusterPrometheusIR = append(multiClusterPrometheusIR, a)
			}
			Expect(multiClusterPrometheusIR).To(HaveLen(4))
			for _, ir := range multiClusterPrometheusIR {
				Expect(ir.SLOs).To(HaveLen(1))
			}
		})
	})

	When("We convert Sloth IR to prometheus rules", func() {
		It("Should create valid MWMB rates", func() {
			var err error
			alertSLO := alert.SLO{
				ID:        "foo",
				Objective: 99.99,
			}
			ctx := context.Background()
			alertGroup, err := slo.GenerateMWWBAlerts(ctx, alertSLO, time.Hour*24, time.Minute*5)
			Expect(err).To(Succeed())
			Expect(alertGroup).To(Not(BeNil()))
		})

		It("Should be able to generate SLI rulefmt.Rules based on alertGroups and SLO definition", func() {
			sampleSLO := simplePrometheusIR[0].SLOs[0]
			ctx := context.Background()
			alertSLO := alert.SLO{
				ID:        "foo",
				Objective: 99.99,
			}
			alertGroup, err := slo.GenerateMWWBAlerts(ctx, alertSLO, time.Hour*24, time.Minute*5)
			Expect(err).To(Succeed())
			rules, err := slo.GenerateSLIRecordingRules(ctx, sampleSLO, *alertGroup)
			Expect(err).To(Succeed())
			// better testing for this when the final format is more stable
			Expect(rules).To(HaveLen(8))

		})

		It("Should be able to generate metadata rulefmt.Rules based on alertGroups and SLO defintion", func() {
			sampleSLO := simplePrometheusIR[0].SLOs[0]
			ctx := context.Background()
			alertSLO := alert.SLO{
				ID:        "foo",
				Objective: 99.99,
			}
			alertGroup, err := slo.GenerateMWWBAlerts(ctx, alertSLO, time.Hour*24, time.Minute*5)
			Expect(err).To(Succeed())
			rules, err := slo.GenerateMetadataRecordingRules(ctx, sampleSLO, alertGroup)
			Expect(err).To(Succeed())
			// better testing for this when the final format is more stable
			Expect(rules).To(HaveLen(7))
		})

		It("Should be able to generate alert rulefmt.Rules base on alertGroups and SLO definition", func() {
			sampleSLO := simplePrometheusIR[0].SLOs[0]
			ctx := context.Background()
			alertSLO := alert.SLO{
				ID:        "foo",
				Objective: 99.99,
			}
			alertGroup, err := slo.GenerateMWWBAlerts(ctx, alertSLO, time.Hour*24, time.Minute*5)
			Expect(err).To(Succeed())
			rules, err := slo.GenerateSLOAlertRules(ctx, sampleSLO, *alertGroup)
			Expect(err).To(Succeed())
			// better testing for this when the final format is more stable
			Expect(rules).To(HaveLen(2))
		})

		It("Should create valid prometheus rules", func() {
			var err error

			for _, sloGroup := range simplePrometheusIR {
				simplePrometheusResponse, err = slo.GeneratePrometheusNoSlothGenerator(
					sloGroup,
					time.Minute*5,
					"", //generate new id
					context.Background(),
					hclog.New(
						&hclog.LoggerOptions{},
					))
				Expect(err).To(Succeed())
				// better testing for this when the final format is more stable
				Expect(len(simplePrometheusResponse)).Should(BeNumerically(">=", 1))
			}

			for _, sloGroup := range objectivePrometheusIR {
				objectivePrometheusResponse, err = slo.GeneratePrometheusNoSlothGenerator(
					sloGroup,
					time.Minute*5,
					"", // generate new id
					context.Background(),
					hclog.New(
						&hclog.LoggerOptions{},
					))
				Expect(err).To(Succeed())
				// better testing for this when the final format is more stable
				Expect(len(objectivePrometheusResponse)).Should(BeNumerically(">=", 1))
			}

			for _, sloGroup := range multiClusterPrometheusIR {
				multiClusterPrometheusResponse, err = slo.GeneratePrometheusNoSlothGenerator(
					sloGroup,
					time.Minute*5,
					"", // generate new id
					context.Background(),
					hclog.New(
						&hclog.LoggerOptions{},
					))
				Expect(err).To(Succeed())
				// better testing for this when the final format is more stable
				Expect(len(multiClusterPrometheusResponse)).Should(BeNumerically(">=", 1))
			}
		})
	})

})
