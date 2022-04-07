package integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni-monitoring/pkg/bootstrap"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/pkp"
	"github.com/rancher/opni-monitoring/pkg/test"
)

//#region Test Setup

type fingerprintsData struct {
	TestData []fingerprintsTestData `json:"testData"`
}

type fingerprintsTestData struct {
	Cert         string             `json:"cert"`
	Fingerprints map[pkp.Alg]string `json:"fingerprints"`
}

var testFingerprints fingerprintsData
var _ = Describe("Agent - Agent and Gateway Bootstrap Tests", Ordered, test.EnableInCI[FlakeAttempts](5), Label(test.Integration, test.Slow, test.TimeSensitive), func() {
	var environment *test.Environment
	var client management.ManagementClient
	var fingerprint string
	BeforeAll(func() {
		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())

		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())
	})

	AfterAll(func() {
		Expect(environment.Stop()).To(Succeed())
	})

	//#endregion

	//#region Happy Path Tests

	var token *core.BootstrapToken
	When("one bootstrap token is available", func() {
		It("should allow an agent to bootstrap using the token", func() {
			var err error
			token, err = client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())

			port, errC := environment.StartAgent("test-cluster-id", token, []string{fingerprint})
			promAgentPort := environment.StartPrometheus(port)
			Expect(promAgentPort).NotTo(BeZero())
			Consistently(errC).ShouldNot(Receive(HaveOccurred()))

		})

		It("should allow multiple agents to bootstrap using the token", func() {
			for i := 0; i < 10; i++ {
				clusterName := "test-cluster-id-" + uuid.New().String()

				_, errC := environment.StartAgent(clusterName, token, []string{fingerprint})
				Consistently(errC).ShouldNot(Receive())
			}

		})

		It("should increment the usage count of the token", func() {
			Eventually(func() int {
				token, err := client.GetBootstrapToken(context.Background(), &core.Reference{
					Id: token.TokenID,
				})
				Expect(err).NotTo(HaveOccurred())
				return int(token.Metadata.UsageCount)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(11))
		})
	})

	//#endregion

	//#region Edge Case Tests

	When("the token is revoked", func() {
		It("should not allow any agents to bootstrap", func() {
			_, err := client.RevokeBootstrapToken(context.Background(), &core.Reference{
				Id: token.TokenID,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				_, err := client.GetBootstrapToken(context.Background(), &core.Reference{
					Id: token.TokenID,
				})
				return err
			}).Should(HaveOccurred())

			clusterName := "test-cluster-id-" + uuid.New().String()

			_, errC := environment.StartAgent(clusterName, token, []string{fingerprint})
			Eventually(errC).Should(Receive(MatchError("bootstrap error: bootstrap failed: 405 Method Not Allowed")))
		})
	})

	When("several agents bootstrap at the same time", func() {
		It("should correctly bootstrap each agent", func() {
			var err error
			token, err = client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())
			ch := make(chan struct{})
			wg := sync.WaitGroup{}
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					<-ch
					clusterName := "test-cluster-id-" + uuid.New().String()

					_, errC := environment.StartAgent(clusterName, token, []string{fingerprint})
					select {
					case err := <-errC:
						Expect(err).NotTo(HaveOccurred())
					case <-time.After(time.Second * 2):
					}

					_, err := client.EditCluster(context.Background(), &management.EditClusterRequest{
						Cluster: &core.Reference{
							Id: clusterName,
						},
						Labels: map[string]string{
							"i": "998",
						},
					})
					Expect(err).NotTo(HaveOccurred())
				}()
			}

			time.Sleep(10 * time.Millisecond) // yield execution to other goroutines
			close(ch)                         // start all goroutines at the same time
			wg.Wait()                         // wait until they all finish

			clusterList, err := client.ListClusters(context.Background(), &management.ListClustersRequest{
				MatchLabels: &core.LabelSelector{
					MatchLabels: map[string]string{
						"i": "998",
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(clusterList.GetItems()).NotTo(BeNil())
			Expect(clusterList.GetItems()).To(HaveLen(10))
		})
		It("should increment the usage count of the token correctly", func() {
			Eventually(func() int {
				tokenUsage, err := client.GetBootstrapToken(context.Background(), &core.Reference{
					Id: token.TokenID,
				})
				Expect(err).NotTo(HaveOccurred())
				return int(tokenUsage.Metadata.UsageCount)
			}, 5*time.Second, 100*time.Millisecond).Should((Equal(10)))
		})
	})

	When("multiple tokens are available", func() {
		It("should allow agents to bootstrap using any token", func() {
			tokens := []*core.BootstrapToken{}
			for i := 0; i < 10; i++ {
				newToken, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
					Ttl: durationpb.New(time.Minute),
				})
				Expect(err).NotTo(HaveOccurred())

				tokens = append(tokens, newToken)
			}

			_, errC := environment.StartAgent("multiple-token-cluster-1", tokens[0], []string{fingerprint})
			Consistently(errC).ShouldNot(Receive())

			time.Sleep(time.Second)
			_, errC = environment.StartAgent("multiple-token-cluster-2", tokens[1], []string{fingerprint})
			Consistently(errC).ShouldNot(Receive())
		})
	})

	When("a token expires but other tokens are available", func() {
		It("should not allow agents to use the expired token", func() {
			exToken, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Second),
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				_, err := client.GetBootstrapToken(context.Background(), &core.Reference{
					Id: exToken.TokenID,
				})
				return err
			}, 10*time.Second, 50*time.Millisecond).Should(HaveOccurred())

			_, errC := environment.StartAgent("multiple-token-cluster-3", exToken, []string{fingerprint})
			Eventually(errC, 5*time.Second).Should(Receive())
		})
	})

	When("a token is created with labels", func() {
		It("should automatically add those labels to any clusters who use it", func() {
			token, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
				Labels: map[string]string{
					"i": "999",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			_, errC := environment.StartAgent("test-cluster-1", token, []string{fingerprint})
			Consistently(errC).ShouldNot(Receive())

			var cluster *core.Cluster
			Eventually(func() (err error) {
				cluster, err = client.GetCluster(context.Background(), &core.Reference{
					Id: "test-cluster-1",
				})
				return
			}).Should(Succeed())

			Expect(cluster.GetLabels()).To(HaveLen(1))
			Expect(cluster.GetLabels()).To(HaveKeyWithValue("i", "999"))
		})
	})

	It("should allow agents to bootstrap using any available certificate", func() {
		token, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		info, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		for _, cert := range info.Chain {
			fp := cert.Fingerprint
			clusterName := "test-cluster-2" + uuid.New().String()

			_, errC := environment.StartAgent(clusterName, token, []string{fp})
			Consistently(errC).ShouldNot(Receive())
		}
	})

	//TODO: This test is not working.
	XWhen("an agent tries to bootstrap twice", func() {
		It("should reject the bootstrap request", func() {
			token, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())

			id := uuid.NewString()
			ctx, cancel := context.WithCancel(context.Background())
			_, errC := environment.StartAgent(id, token, []string{fingerprint}, test.WithContext(ctx))

			Consistently(errC).ShouldNot(Receive())
			cancel()

			etcdClient, err := environment.EtcdClient()
			Expect(err).NotTo(HaveOccurred())
			defer etcdClient.Close()

			_, err = etcdClient.Delete(context.Background(), "/agent/keyrings/"+id)
			Expect(err).NotTo(HaveOccurred())

			_, errC = environment.StartAgent(id, token, []string{fingerprint})

			Eventually(errC).Should(Receive(MatchError(bootstrap.ErrBootstrapFailed)))
		})
	})

	When("an agent requests an ID that is already in use", func() {
		var prevUsageCount int32
		It("should reject the bootstrap request", func() {
			tokenUsage, err := client.GetBootstrapToken(context.Background(), &core.Reference{
				Id: token.TokenID,
			})
			Expect(err).NotTo(HaveOccurred())
			prevUsageCount = int32(tokenUsage.Metadata.UsageCount)

			tempToken, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())

			_, errC := environment.StartAgent("test-cluster-3", tempToken, []string{fingerprint})
			Consistently(errC).ShouldNot(Receive())
			Eventually(func() error {
				_, err := client.GetCluster(context.Background(), &core.Reference{
					Id: "test-cluster-3",
				})
				return err
			}).Should(Succeed())

			Eventually(environment.GetAgent("test-cluster-3")).ShouldNot(BeNil())
			Expect(environment.GetAgent("test-cluster-3").Shutdown()).To(Succeed())

			etcdClient, err := environment.EtcdClient()
			Expect(err).NotTo(HaveOccurred())
			defer etcdClient.Close()

			resp, err := etcdClient.Delete(context.Background(), "agent/keyrings/test-cluster-3")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Deleted).To(BeEquivalentTo(1))

			_, errC = environment.StartAgent("test-cluster-3", token, []string{fingerprint})
			Eventually(errC).Should(Receive(MatchError("bootstrap error: bootstrap failed: bootstrap failed: 409 Conflict")))
		})

		It("should not increment the usage count of the token ", func() {
			tokenUsage, err := client.GetBootstrapToken(context.Background(), &core.Reference{
				Id: token.TokenID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenUsage.Metadata.UsageCount).To(BeEquivalentTo(prevUsageCount))
		})
	})

	//#endregion
})
