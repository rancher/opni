package metrics_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/rancher/opni/pkg/alerting/metrics/naming"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/tidwall/gjson"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/test"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type TestSeriesMetrics struct {
	input  *cortexadmin.SeriesRequest
	output *cortexadmin.SeriesInfoList
}

type TestMetricLabelSet struct {
	input  *cortexadmin.LabelRequest
	output *cortexadmin.MetricLabels
}

func expectRuleGroupToExist(ctx context.Context, adminClient cortexadmin.CortexAdminClient, tenant string, groupName string, expectedYaml []byte) error {
	resp, err := adminClient.GetRule(ctx, &cortexadmin.GetRuleRequest{
		ClusterId: tenant,
		Namespace: "test",
		GroupName: groupName,
	})
	if err == nil {
		Expect(resp.Data).To(Not(BeNil()))
		Expect(resp.Data).To(MatchYAML(expectedYaml))
		return nil
	}
	return fmt.Errorf("Rule %s should exist, but doesn't", groupName)
}

func expectRuleGroupToNotExist(ctx context.Context, adminClient cortexadmin.CortexAdminClient, tenant string, groupName string) error {
	_, err := adminClient.GetRule(ctx, &cortexadmin.GetRuleRequest{
		ClusterId: tenant,
		Namespace: "test",
		GroupName: groupName,
	})
	if err != nil {
		Expect(status.Code(err)).To(Equal(codes.NotFound))
		return nil
	}

	return fmt.Errorf("Rule %s still exists, but shouldn't", groupName)
}

type mockPod struct {
	podName   string
	namespace string
	phase     string
	uid       string
}

func setMockKubernetesPodState(kubePort int, pod *mockPod) {
	queryUrl := fmt.Sprintf("http://localhost:%d/set", kubePort)
	client := &http.Client{
		Transport: &http.Transport{},
	}
	req, err := http.NewRequest("GET", queryUrl, nil)
	if err != nil {
		panic(err)
	}
	values := url.Values{}
	values.Set("obj", "pod")
	values.Set("name", pod.podName)
	values.Set("namespace", pod.namespace)
	values.Set("phase", pod.phase)
	values.Set("uid", pod.uid)
	req.URL.RawQuery = values.Encode()
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			panic(fmt.Sprintf("kube metrics prometheus collector hit an error %d", resp.StatusCode))
		}
	}()
}

var _ = XDescribe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label("integration", "slow"), func() {
	ctx := context.Background()
	var env *test.Environment
	var adminClient cortexadmin.CortexAdminClient
	var kubernetesTempMetricServerPort int
	var kubernetesJobName string
	ruleTestDataDir := "../../../pkg/test/testdata/testdata/slo/cortexrule"
	BeforeAll(func() {
		env = &test.Environment{}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop, "Test Suite Finished")

		opsClient := cortexops.NewCortexOpsClient(env.ManagementClientConn())
		err := cortexops.InstallWithPreset(env.Context(), opsClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(cortexops.WaitForReady(env.Context(), opsClient)).To(Succeed())

		client := env.NewManagementClient()

		adminClient = cortexadmin.NewCortexAdminClient(env.ManagementClientConn())
		// wait until data has been stored in cortex for the cluster
		kubernetesTempMetricServerPort = env.StartMockKubernetesMetricServer()
		fmt.Printf("Mock kubernetes metrics server started on port %d\n", kubernetesTempMetricServerPort)
		err = env.BootstrapNewAgent("agent")
		Expect(err).NotTo(HaveOccurred())
		kubernetesJobName = "kubernetes"
		env.SetPrometheusNodeConfigOverride("agent", test.NewOverridePrometheusConfig(
			"alerting/prometheus/config.yaml",
			[]test.PrometheusJob{
				{
					JobName:    kubernetesJobName,
					ScrapePort: kubernetesTempMetricServerPort,
				},
			}),
		)
		err = env.BootstrapNewAgent("agent2")
		Expect(err).NotTo(HaveOccurred())

		_, err = client.InstallCapability(context.Background(), &capabilityv1.InstallRequest{
			Capability: &corev1.Reference{Id: wellknown.CapabilityMetrics},
			Agent:      &corev1.Reference{Id: "agent"},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.InstallCapability(context.Background(), &capabilityv1.InstallRequest{
			Capability: &corev1.Reference{Id: wellknown.CapabilityMetrics},
			Agent:      &corev1.Reference{Id: "agent2"},
		})
		Expect(err).NotTo(HaveOccurred())

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
		}, 60*time.Second, 1*time.Second).Should(Succeed())

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
		}, 60*time.Second, 1*time.Second).Should(Succeed())

		//scrape interval is 1 second
	})

	When("We use the cortex admin plugin", func() {
		It("Should be able to enumerate all kube metrics series that match a certain expression", func() {
			setMockKubernetesPodState(kubernetesTempMetricServerPort, &mockPod{
				podName:   "test-pod",
				namespace: "test-namespace",
				phase:     "Running",
				uid:       "test-uid",
			})
			Eventually(func() error {
				resp, err := adminClient.ExtractRawSeries(ctx, &cortexadmin.MatcherRequest{
					Tenant:    "agent",
					MatchExpr: naming.KubeObjMetricNameMatcher,
				},
				)
				if err != nil {
					return err
				}

				result := gjson.Get(string(resp.Data), "data.result")
				if !result.Exists() {
					return fmt.Errorf("no result data")
				}
				if len(result.Array()) == 0 {
					return fmt.Errorf("no results")
				}
				return nil
			}, 3*time.Minute, 30*time.Second).Should(Succeed())

		})

		It("Should be able to list distinct metrics for each job ", func() {
			// expected outputs are a subset of the actual outputs
			inputs := []TestSeriesMetrics{
				{
					input: &cortexadmin.SeriesRequest{
						Tenant: "agent",
						JobId:  "prometheus",
					},
					output: &cortexadmin.SeriesInfoList{
						Items: []*cortexadmin.SeriesInfo{
							{
								SeriesName: "up",
							},
							{
								SeriesName: "prometheus_http_requests_total",
							},
						},
					},
				},
				{
					input: &cortexadmin.SeriesRequest{
						Tenant: "agent2",
						JobId:  "prometheus",
					},
					output: &cortexadmin.SeriesInfoList{
						Items: []*cortexadmin.SeriesInfo{
							{
								SeriesName: "up",
							},
							{
								SeriesName: "prometheus_http_requests_total",
							},
						},
					},
				},
			}
			for _, input := range inputs {
				resp, err := adminClient.GetSeriesMetrics(ctx, input.input)
				Expect(err).NotTo(HaveOccurred())
				for _, expected := range input.output.Items {
					found := false
					for _, item := range resp.Items {
						if item.SeriesName == expected.SeriesName {
							//FIXME: when the metadata API is working also check metadata here
							found = true
							break
						}
					}
					Expect(found).To(BeTrue())
				}
			}
		})

		It("should be able to fetch metric label pairs for each metric", func() {
			// expected outputs are a subset of the actual outputs
			inputs := []TestMetricLabelSet{
				{
					input: &cortexadmin.LabelRequest{
						Tenant:     "agent",
						JobId:      "prometheus",
						MetricName: "prometheus_http_requests_total",
					},
					output: &cortexadmin.MetricLabels{
						Items: []*cortexadmin.LabelSet{
							{
								Name: "code",
								Items: []string{
									"200",
								},
							},
							{
								Name: "handler",
								Items: []string{
									"/-/ready",
									"/metrics",
								},
							},
						},
					},
				},
				{
					input: &cortexadmin.LabelRequest{
						Tenant:     "agent2",
						JobId:      "prometheus",
						MetricName: "prometheus_http_requests_total",
					},
					output: &cortexadmin.MetricLabels{
						Items: []*cortexadmin.LabelSet{
							{
								Name: "code",
								Items: []string{
									"200",
								},
							},
							{
								Name: "handler",
								Items: []string{
									"/-/ready",
								},
							},
						},
					},
				},
			}
			for _, input := range inputs {
				resp, err := adminClient.GetMetricLabelSets(ctx, input.input)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				for _, expected := range input.output.Items {
					found := false
					for _, item := range resp.Items {
						if item.Name == expected.Name {
							found = true
							for _, expectedItem := range expected.Items {
								foundItem := false
								for _, item := range item.Items {
									if item == expectedItem {
										foundItem = true
										break
									}
								}
								Expect(foundItem).To(BeTrue())
							}
							break
						}
					}
					Expect(found).To(BeTrue())
				}
			}
		})

		It("Should be able to create rules from prometheus yaml", func() {
			sampleRule := fmt.Sprintf("%s/sampleRule.yaml", ruleTestDataDir)
			sampleRuleYamlString, err := os.ReadFile(sampleRule)
			Expect(err).To(Succeed())
			_, err = adminClient.LoadRules(ctx,
				&cortexadmin.LoadRuleRequest{
					ClusterId:   "agent",
					Namespace:   "test",
					YamlContent: sampleRuleYamlString,
				})
			Expect(err).To(Succeed())

			// Note that sloth by default groups its output into a list of rulefmt.RuleGroup called "groups:"
			// While we require the list of rulefmt.RuleGroup to be separated by "---\n"
			slothGeneratedGroup := fmt.Sprintf("%s/slothGeneratedGroup.yaml", ruleTestDataDir)
			slothGeneratedGroupYamlString, err := os.ReadFile(slothGeneratedGroup)
			Expect(err).To(Succeed())
			_, err = adminClient.LoadRules(ctx,
				&cortexadmin.LoadRuleRequest{
					ClusterId:   "agent",
					Namespace:   "test",
					YamlContent: slothGeneratedGroupYamlString,
				})
			Expect(err).To(Succeed())
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				return expectRuleGroupToExist(
					ctx, adminClient, "agent",
					"opni-test-slo-rule", sampleRuleYamlString)
			}).Should(Succeed())

		})

		It("Should be able to update existing rule groups", func() {
			sampleRuleUpdate := fmt.Sprintf("%s/sampleRuleUpdate.yaml", ruleTestDataDir)
			sampleRuleYamlUpdateString, err := os.ReadFile(sampleRuleUpdate)
			Expect(err).To(Succeed())
			_, err = adminClient.LoadRules(ctx,
				&cortexadmin.LoadRuleRequest{
					ClusterId:   "agent",
					Namespace:   "test",
					YamlContent: sampleRuleYamlUpdateString,
				})
			Expect(err).To(Succeed())

			Eventually(func() error {
				return expectRuleGroupToExist(
					ctx, adminClient, "agent",
					"opni-test-slo-rule", sampleRuleYamlUpdateString)
			}).Should(Succeed())
		})

		It("Should be able to delete existing rule groups", func() {
			deleteGroupName := "opni-test-slo-rule"
			_, err := adminClient.DeleteRule(ctx, &cortexadmin.DeleteRuleRequest{
				ClusterId: "agent",
				Namespace: "test",
				GroupName: deleteGroupName,
			})
			Expect(err).To(Succeed())

			// Should find no rule named "opni-test-slo-rule" after deletion
			Eventually(func() error {
				return expectRuleGroupToNotExist(
					ctx, adminClient, "agent",
					"opni-test-slo-rule")
			}).Should(Succeed())
		})
	})
	When("We are in a multitenant environment", func() {
		It("Should be able to apply rules across tenants", func() {
			sampleRule := fmt.Sprintf("%s/sampleRule.yaml", ruleTestDataDir)
			sampleRuleYamlString, err := os.ReadFile(sampleRule)
			Expect(err).To(Succeed())
			_, err = adminClient.LoadRules(ctx,
				&cortexadmin.LoadRuleRequest{
					ClusterId:   "agent",
					Namespace:   "test",
					YamlContent: sampleRuleYamlString,
				})
			Expect(err).To(Succeed())
			_, err = adminClient.LoadRules(ctx,
				&cortexadmin.LoadRuleRequest{
					Namespace:   "test",
					ClusterId:   "agent2",
					YamlContent: sampleRuleYamlString,
				})
			Expect(err).To(Succeed())

			Eventually(func() error {
				return expectRuleGroupToExist(
					ctx, adminClient,
					"agent", "opni-test-slo-rule", sampleRuleYamlString)
			}).Should(Succeed())

			Eventually(func() error {
				return expectRuleGroupToExist(
					ctx, adminClient,
					"agent2", "opni-test-slo-rule", sampleRuleYamlString,
				)
			}).Should(Succeed())

			deleteGroupName := "opni-test-slo-rule"
			_, err = adminClient.DeleteRule(ctx, &cortexadmin.DeleteRuleRequest{
				ClusterId: "agent",
				Namespace: "test",
				GroupName: deleteGroupName,
			})
			Expect(err).To(Succeed())

			Eventually(func() error {
				return expectRuleGroupToExist(
					ctx, adminClient, "agent2",
					"opni-test-slo-rule", sampleRuleYamlString)
			}).Should(Succeed())

			Eventually(func() error {
				return expectRuleGroupToNotExist(
					ctx, adminClient, "agent",
					"opni-test-slo-rule")
			}).Should(Succeed())
		})
	})
})
