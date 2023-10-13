package metrics_test

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Node Config", Ordered, Label("integration"), func() {
	var mgmtClient managementv1.ManagementClient
	var nodeClient node.NodeConfigurationClient
	BeforeAll(func() {
		env := &test.Environment{}
		Expect(env.Start()).To(Succeed())

		mgmtClient = env.NewManagementClient()

		token, err := mgmtClient.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		certs, err := mgmtClient.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		fp := certs.Chain[len(certs.Chain)-1].Fingerprint

		_, errC := env.StartAgent("agent1", token, []string{fp}, test.WithAgentVersion("v2"), test.WithLocalAgent())
		Eventually(errC).Should(Receive(BeNil()))

		_, errC = env.StartAgent("agent2", token, []string{fp}, test.WithAgentVersion("v2"))
		Eventually(errC).Should(Receive(BeNil()))

		nodeClient = node.NewNodeConfigurationClient(env.ManagementClientConn())
		DeferCleanup(env.Stop, "Test Suite Finished")
	})

	var getConfig = func(agentId string) (*node.MetricsCapabilityConfig, error) {
		spec, err := nodeClient.GetConfiguration(context.Background(), &node.GetRequest{
			Node: &v1.Reference{Id: agentId},
		})
		if err != nil {
			return nil, err
		}
		driverutil.UnsetRevision(spec) // todo: update these (older) tests to consider revisions
		return spec, nil
	}

	var verifySync = func(fn func(), capability string, agentIds ...string) {
		GinkgoHelper()
		times := make(map[string]time.Time)
		for _, id := range agentIds {
			id := strings.TrimPrefix(id, "!")
			req := &managementv1.CapabilityStatusRequest{
				Name:    capability,
				Cluster: &v1.Reference{Id: id},
			}
			Eventually(func() bool {
				stat, err := mgmtClient.CapabilityStatus(context.Background(), req)
				if status.Code(err) == codes.NotFound {
					return false
				} else {
					Expect(err).NotTo(HaveOccurred())
					times[id] = stat.GetLastSync().AsTime()
				}
				return true
			}).Should(BeTrue(), "failed to get sync time for %s", id)
		}

		fn()

		for _, id := range agentIds {
			expectNoUpdate := strings.HasPrefix(id, "!")
			id := strings.TrimPrefix(id, "!")
			req := &managementv1.CapabilityStatusRequest{
				Name:    capability,
				Cluster: &v1.Reference{Id: id},
			}

			if expectNoUpdate {
				Consistently(func() bool {
					stat, err := mgmtClient.CapabilityStatus(context.Background(), req)
					if status.Code(err) == codes.NotFound {
						return false
					} else {
						Expect(err).NotTo(HaveOccurred())
					}
					return stat.GetLastSync().AsTime().Equal(times[id])
				}).Should(BeTrue(), "expected sync time not to be updated for agent %s", id)
				continue
			} else {
				Eventually(func() bool {
					stat, err := mgmtClient.CapabilityStatus(context.Background(), req)
					if status.Code(err) == codes.NotFound {
						return false
					} else {
						Expect(err).NotTo(HaveOccurred())
					}
					syncTime := times[id]
					return stat.GetLastSync().AsTime().After(syncTime)
				}).Should(BeTrue(), "expected sync time to be updated for agent %s", id)
			}
		}
	}

	var originalDefaultConfig *node.MetricsCapabilityConfig
	It("should initially have all nodes using the default config", func() {
		defaultConfig, err := nodeClient.GetDefaultConfiguration(context.Background(), &node.GetRequest{})
		Expect(err).NotTo(HaveOccurred())
		driverutil.UnsetRevision(defaultConfig)
		originalDefaultConfig = defaultConfig

		spec, err := getConfig("agent1")
		Expect(err).NotTo(HaveOccurred())
		Expect(spec).To(testutil.ProtoEqual(defaultConfig))

		spec, err = getConfig("agent2")
		Expect(err).NotTo(HaveOccurred())
		Expect(spec).To(testutil.ProtoEqual(defaultConfig))
	})

	When("changing the default config", func() {
		It("should return the new config for all nodes", func() {
			newConfig := &node.MetricsCapabilityConfig{
				Driver: lo.ToPtr(node.MetricsCapabilityConfig_Prometheus),
				Prometheus: &node.PrometheusSpec{
					Image: lo.ToPtr("foo"),
				},
			}

			verifySync(func() {
				_, err := nodeClient.SetDefaultConfiguration(context.Background(), &node.SetRequest{
					Spec: newConfig,
				})
				Expect(err).NotTo(HaveOccurred())
			}, "metrics", "agent1", "agent2")

			spec, err := getConfig("agent1")
			Expect(err).NotTo(HaveOccurred())

			Expect(spec).To(testutil.ProtoEqual(newConfig))

			spec, err = getConfig("agent2")
			Expect(err).NotTo(HaveOccurred())
			Expect(spec).To(testutil.ProtoEqual(newConfig))
		})
	})

	When("setting a config for a node", func() {
		It("should return the new config for that node", func() {
			newConfig := &node.MetricsCapabilityConfig{
				Driver: lo.ToPtr(node.MetricsCapabilityConfig_Prometheus),
				Prometheus: &node.PrometheusSpec{
					Image: lo.ToPtr("bar"),
				},
			}

			defaultConfig, err := nodeClient.GetDefaultConfiguration(context.Background(), &node.GetRequest{})
			Expect(err).NotTo(HaveOccurred())
			driverutil.UnsetRevision(defaultConfig)

			verifySync(func() {
				_, err = nodeClient.SetConfiguration(context.Background(), &node.SetRequest{
					Node: &v1.Reference{Id: "agent1"},
					Spec: newConfig,
				})
				Expect(err).NotTo(HaveOccurred())
			}, "metrics", "agent1", "!agent2")

			spec, err := getConfig("agent1")
			Expect(err).NotTo(HaveOccurred())
			Expect(spec).To(testutil.ProtoEqual(newConfig))

			spec, err = getConfig("agent2")
			Expect(err).NotTo(HaveOccurred())
			Expect(spec).To(testutil.ProtoEqual(defaultConfig))
		})
	})

	When("resetting a config for a node", func() {
		It("should return the default config for that node", func() {
			defaultConfig, err := nodeClient.GetDefaultConfiguration(context.Background(), &node.GetRequest{})
			Expect(err).NotTo(HaveOccurred())
			driverutil.UnsetRevision(defaultConfig)

			verifySync(func() {
				_, err = nodeClient.ResetConfiguration(context.Background(), &node.ResetRequest{
					Node: &v1.Reference{Id: "agent1"},
				})
				Expect(err).NotTo(HaveOccurred())
			}, "metrics", "agent1", "!agent2")

			spec, err := getConfig("agent1")
			Expect(err).NotTo(HaveOccurred())
			Expect(spec).To(testutil.ProtoEqual(defaultConfig))

			spec, err = getConfig("agent2")
			Expect(err).NotTo(HaveOccurred())
			Expect(spec).To(testutil.ProtoEqual(defaultConfig))
		})
	})

	When("resetting the default config", func() {
		It("should return the original default config for all nodes", func() {
			verifySync(func() {
				_, err := nodeClient.ResetDefaultConfiguration(context.Background(), &emptypb.Empty{})
				Expect(err).NotTo(HaveOccurred())
			}, "metrics", "agent1", "agent2")

			spec, err := getConfig("agent1")
			Expect(err).NotTo(HaveOccurred())
			Expect(spec).To(testutil.ProtoEqual(originalDefaultConfig))

			spec, err = getConfig("agent2")
			Expect(err).NotTo(HaveOccurred())
			Expect(spec).To(testutil.ProtoEqual(originalDefaultConfig))
		})
	})

	When("setting a config for a node that is the same as the default", func() {
		It("should preserve the config for that node if the default changes", func() {
			defaultConfig, err := nodeClient.GetDefaultConfiguration(context.Background(), &node.GetRequest{})
			Expect(err).NotTo(HaveOccurred())
			driverutil.UnsetRevision(defaultConfig)

			verifySync(func() {
				_, err = nodeClient.SetConfiguration(context.Background(), &node.SetRequest{
					Node: &v1.Reference{Id: "agent1"},
					Spec: defaultConfig,
				})
				Expect(err).NotTo(HaveOccurred())
			}, "metrics", "agent1", "!agent2")

			newConfig := &node.MetricsCapabilityConfig{
				Driver: lo.ToPtr(node.MetricsCapabilityConfig_Prometheus),
				Prometheus: &node.PrometheusSpec{
					Image: lo.ToPtr("foo"),
				},
			}
			verifySync(func() {
				_, err = nodeClient.SetDefaultConfiguration(context.Background(), &node.SetRequest{
					Spec: newConfig,
				})
				Expect(err).NotTo(HaveOccurred())
			}, "metrics", "agent1", "agent2")

			spec, err := getConfig("agent1")
			Expect(spec).To(testutil.ProtoEqual(defaultConfig))

			spec, err = getConfig("agent2")
			Expect(spec).To(testutil.ProtoEqual(newConfig))
		})
	})
})
