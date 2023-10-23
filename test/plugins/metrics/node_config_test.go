package metrics_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type nodeConfigTestData struct {
	mgmtClient managementv1.ManagementClient
}

var _ = Describe("Node Config", Ordered, Label("integration"), NodeConfigTestSuite(func() nodeConfigTestData {
	env := &test.Environment{}
	Expect(env.Start()).To(Succeed())

	mgmtClient := env.NewManagementClient()

	err := env.BootstrapNewAgent("agent1", test.WithLocalAgent())
	Expect(err).NotTo(HaveOccurred())

	err = env.BootstrapNewAgent("agent2")
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(env.Stop, "Test Suite Finished")
	return nodeConfigTestData{
		mgmtClient: mgmtClient,
	}
}))

var _ = Describe("Node Config (HA)", Ordered, Label("integration"), NodeConfigTestSuite(func() nodeConfigTestData {
	By("Starting 2 test environments")
	env1 := &test.Environment{
		Logger: testlog.Log.WithGroup("env1"),
	}
	Expect(env1.Start(test.WithStorageBackend(v1beta1.StorageTypeEtcd))).To(Succeed())
	env2 := &test.Environment{
		Logger: testlog.Log.WithGroup("env2"),
	}
	Expect(env2.Start(test.WithStorageBackend(v1beta1.StorageTypeEtcd), test.WithRemoteEtcdPort(env1.GetPorts().Etcd))).To(Succeed())

	resolver := test.NewEnvironmentResolver(env1, env2)
	cc, err := grpc.Dial("testenv:///management", grpc.WithResolvers(resolver), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.WaitForReady(true)))
	Expect(err).NotTo(HaveOccurred())
	mgmtClient := managementv1.NewManagementClient(cc)

	By("adding one agent to each environment")
	err = env1.BootstrapNewAgent("agent1", test.WithLocalAgent())
	Expect(err).NotTo(HaveOccurred())

	err = env2.BootstrapNewAgent("agent2")
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(env1.Stop, "Test Suite Finished")
	return nodeConfigTestData{
		mgmtClient: mgmtClient,
	}
}))

func NodeConfigTestSuite(setup func() nodeConfigTestData) func() {
	return func() {
		var mgmtClient managementv1.ManagementClient
		var nodeClient node.NodeConfigurationClient
		BeforeAll(func() {
			data := setup()
			mgmtClient = data.mgmtClient
			nodeClient = node.NewNodeConfigurationClient(managementv1.UnderlyingConn(mgmtClient))
		})

		var getConfig = func(agentId string) (*node.MetricsCapabilityConfig, error) {
			spec, err := nodeClient.GetConfiguration(context.Background(), &node.GetRequest{
				Node: &corev1.Reference{Id: agentId},
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
				req := &capabilityv1.StatusRequest{
					Capability: &corev1.Reference{Id: capability},
					Agent:      &corev1.Reference{Id: id},
				}
				Eventually(func() error {
					stat, err := mgmtClient.CapabilityStatus(context.Background(), req)
					if status.Code(err) == codes.NotFound {
						return err
					} else {
						Expect(err).NotTo(HaveOccurred())
						times[id] = stat.GetLastSync().AsTime()
					}
					return nil
				}).Should(Succeed(), "failed to get sync time for %s", id)
			}

			fn()

			for _, id := range agentIds {
				expectNoUpdate := strings.HasPrefix(id, "!")
				id := strings.TrimPrefix(id, "!")
				req := &capabilityv1.StatusRequest{
					Capability: &corev1.Reference{Id: capability},
					Agent:      &corev1.Reference{Id: id},
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
					Eventually(func() error {
						stat, err := mgmtClient.CapabilityStatus(context.Background(), req)
						if status.Code(err) == codes.NotFound {
							return err
						} else {
							Expect(err).NotTo(HaveOccurred())
						}
						syncTime := times[id]
						if stat.GetLastSync().AsTime().After(syncTime) {
							return nil
						} else {
							return fmt.Errorf("sync time not updated for agent %s", id)
						}
					}).Should(Succeed(), "expected sync time to be updated for agent %s", id)
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
				config, err := nodeClient.GetDefaultConfiguration(context.Background(), &node.GetRequest{})
				Expect(err).NotTo(HaveOccurred())
				config.Driver = lo.ToPtr(node.MetricsCapabilityConfig_Prometheus)
				config.Prometheus = &node.PrometheusSpec{
					Image: lo.ToPtr("foo"),
				}

				verifySync(func() {
					_, err := nodeClient.SetDefaultConfiguration(context.Background(), &node.SetRequest{
						Spec: config,
					})
					Expect(err).NotTo(HaveOccurred())
				}, "metrics", "agent1", "agent2")

				driverutil.UnsetRevision(config)

				spec, err := getConfig("agent1")
				Expect(err).NotTo(HaveOccurred())

				Expect(spec).To(testutil.ProtoEqual(config))

				spec, err = getConfig("agent2")
				Expect(err).NotTo(HaveOccurred())
				Expect(spec).To(testutil.ProtoEqual(config))
			})
		})

		When("setting a config for a node", func() {
			It("should return the new config for that node", func() {
				defaultConfig, err := nodeClient.GetDefaultConfiguration(context.Background(), &node.GetRequest{})
				Expect(err).NotTo(HaveOccurred())
				driverutil.UnsetRevision(defaultConfig)

				config, err := nodeClient.GetDefaultConfiguration(context.Background(), &node.GetRequest{})
				Expect(err).NotTo(HaveOccurred())
				config.Driver = lo.ToPtr(node.MetricsCapabilityConfig_Prometheus)
				config.Prometheus = &node.PrometheusSpec{
					Image: lo.ToPtr("bar"),
				}

				verifySync(func() {
					_, err = nodeClient.SetConfiguration(context.Background(), &node.SetRequest{
						Node: &corev1.Reference{Id: "agent1"},
						Spec: config,
					})
					Expect(err).NotTo(HaveOccurred())
				}, "metrics", "agent1", "!agent2")

				driverutil.UnsetRevision(config)
				spec, err := getConfig("agent1")
				Expect(err).NotTo(HaveOccurred())
				Expect(spec).To(testutil.ProtoEqual(config))

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
						Node: &corev1.Reference{Id: "agent1"},
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

				verifySync(func() {
					_, err = nodeClient.SetConfiguration(context.Background(), &node.SetRequest{
						Node: &corev1.Reference{Id: "agent1"},
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
				driverutil.UnsetRevision(defaultConfig)
				Expect(spec).To(testutil.ProtoEqual(defaultConfig))

				spec, err = getConfig("agent2")
				Expect(spec).To(testutil.ProtoEqual(newConfig))
			})
		})
	}
}
