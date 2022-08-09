package plugins_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	apis "github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func expectRuleGroupToExist(adminClient apis.CortexAdminClient, ctx context.Context, tenant string, groupName string, expectedYaml []byte) error {
	for i := 0; i < 10; i++ {
		resp, err := adminClient.GetRule(ctx, &apis.RuleRequest{
			ClusterId: tenant,
			GroupName: groupName,
		})
		if err == nil {
			Expect(resp.Data).To(Not(BeNil()))
			Expect(resp.Data).To(MatchYAML(expectedYaml))
			return nil
		}
		time.Sleep(1)
	}
	return fmt.Errorf("Rule %s should exist, but doesn't", groupName)
}

func expectRuleGroupToNotExist(adminClient apis.CortexAdminClient, ctx context.Context, tenant string, groupName string) error {
	for i := 0; i < 10; i++ {
		_, err := adminClient.GetRule(ctx, &apis.RuleRequest{
			ClusterId: tenant,
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

var _ = Describe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	var env *test.Environment
	var adminClient apis.CortexAdminClient
	ruleTestDataDir := "../../../pkg/test/testdata/slo/cortexrule"
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
		adminClient = apis.NewCortexAdminClient(env.ManagementClientConn())
		// wait until data has been stored in cortex for the cluster
		adminClient := env.NewCortexAdminClient()

		p, errc := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		Eventually(errc).Should(Receive(BeNil()))
		env.StartPrometheus(p)
		p2, errc2 := env.StartAgent("agent2", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		Eventually(errc2).Should(Receive(BeNil()))
		env.StartPrometheus(p2)
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

		//scrape interval is 1 second
	})

	When("We use the cortex admin plugin", func() {

		It("Should be able to fetch series metadata successfully", func() {
			inputs := []TestMetadataInput{
				{
					Tenant:     "agent",
					MetricName: "http_requests_duration_seconds_total",
				},
				{
					Tenant:     "agent2",
					MetricName: "http_requests_duration_seconds_total",
				},
			}
			for _, input := range inputs {
				_, err := adminClient.GetSeriesMetadata(ctx, &apis.SeriesRequest{
					Tenant:     input.Tenant,
					MetricName: input.MetricName,
				})
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Should be able to fetch a metric's label values", func() {
			inputs := []TestMetadataInput{
				{
					Tenant:     "agent",
					MetricName: "prometheus",
				},
				{
					Tenant:     "agent2",
					MetricName: "prometheus",
				},
			}
			for _, input := range inputs {
				_, err := adminClient.GetMetricLabels(ctx, &apis.SeriesRequest{
					Tenant:     input.Tenant,
					MetricName: input.MetricName,
				})
				Expect(err).NotTo(HaveOccurred())
			}

		})

		It("Should be able to create rules from prometheus yaml", func() {
			sampleRule := fmt.Sprintf("%s/sampleRule.yaml", ruleTestDataDir)
			sampleRuleYamlString, err := ioutil.ReadFile(sampleRule)
			Expect(err).To(Succeed())
			_, err = adminClient.LoadRules(ctx,
				&apis.PostRuleRequest{
					ClusterId:   "agent",
					YamlContent: string(sampleRuleYamlString),
				})
			Expect(err).To(Succeed())

			// Note that sloth by default groups its output into a list of rulefmt.RuleGroup called "groups:"
			// While we require the list of rulefmt.RuleGroup to be separated by "---\n"
			slothGeneratedGroup := fmt.Sprintf("%s/slothGeneratedGroup.yaml", ruleTestDataDir)
			slothGeneratedGroupYamlString, err := ioutil.ReadFile(slothGeneratedGroup)
			Expect(err).To(Succeed())
			_, err = adminClient.LoadRules(ctx,
				&apis.PostRuleRequest{
					ClusterId:   "agent",
					YamlContent: string(slothGeneratedGroupYamlString),
				})
			Expect(err).To(Succeed())
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				return expectRuleGroupToExist(
					adminClient, ctx, "agent",
					"opni-test-slo-rule", sampleRuleYamlString)
			}).Should(Succeed())

		})

		It("Should be able to update existing rule groups", func() {
			sampleRuleUpdate := fmt.Sprintf("%s/sampleRuleUpdate.yaml", ruleTestDataDir)
			sampleRuleYamlUpdateString, err := ioutil.ReadFile(sampleRuleUpdate)
			Expect(err).To(Succeed())
			_, err = adminClient.LoadRules(ctx,
				&apis.PostRuleRequest{
					ClusterId:   "agent",
					YamlContent: string(sampleRuleYamlUpdateString),
				})
			Expect(err).To(Succeed())

			Eventually(func() error {
				return expectRuleGroupToExist(
					adminClient, ctx, "agent",
					"opni-test-slo-rule", sampleRuleYamlUpdateString)
			}).Should(Succeed())
		})

		It("Should be able to delete existing rule groups", func() {
			deleteGroupName := "opni-test-slo-rule"
			_, err := adminClient.DeleteRule(ctx, &apis.RuleRequest{
				ClusterId: "agent",
				GroupName: deleteGroupName,
			})
			Expect(err).To(Succeed())

			// Should find no rule named "opni-test-slo-rule" after deletion
			Eventually(func() error {
				return expectRuleGroupToNotExist(
					adminClient, ctx, "agent",
					"opni-test-slo-rule")
			}).Should(Succeed())
		})
	})
	When("We are in a multitenant environment", func() {
		It("Should be able to apply rules across tenants", func() {
			sampleRule := fmt.Sprintf("%s/sampleRule.yaml", ruleTestDataDir)
			sampleRuleYamlString, err := ioutil.ReadFile(sampleRule)
			Expect(err).To(Succeed())
			_, err = adminClient.LoadRules(ctx,
				&apis.PostRuleRequest{
					ClusterId:   "agent",
					YamlContent: string(sampleRuleYamlString),
				})
			Expect(err).To(Succeed())
			_, err = adminClient.LoadRules(ctx,
				&apis.PostRuleRequest{
					ClusterId:   "agent2",
					YamlContent: string(sampleRuleYamlString),
				})
			Expect(err).To(Succeed())

			Eventually(func() error {
				return expectRuleGroupToExist(
					adminClient, ctx,
					"agent", "opni-test-slo-rule", sampleRuleYamlString)
			}).Should(Succeed())

			Eventually(func() error {
				return expectRuleGroupToExist(
					adminClient, ctx,
					"agent2", "opni-test-slo-rule", sampleRuleYamlString,
				)
			}).Should(Succeed())

			deleteGroupName := "opni-test-slo-rule"
			_, err = adminClient.DeleteRule(ctx, &apis.RuleRequest{
				ClusterId: "agent",
				GroupName: deleteGroupName,
			})
			Expect(err).To(Succeed())

			Eventually(func() error {
				return expectRuleGroupToExist(
					adminClient, ctx, "agent2",
					"opni-test-slo-rule", sampleRuleYamlString)
			}).Should(Succeed())

			Eventually(func() error {
				return expectRuleGroupToNotExist(
					adminClient, ctx, "agent",
					"opni-test-slo-rule")
			}).Should(Succeed())
		})
	})
})
