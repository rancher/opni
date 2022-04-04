package integration_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/test"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

//#region Test Setup
var _ = Describe("Management API Cluster Management Tests", Ordered, Label(test.Integration), func() {
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

		token, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())

		port, errC := environment.StartAgent("test-cluster-id", token, []string{fingerprint})
		promAgentPort := environment.StartPrometheus(port)
		Expect(promAgentPort).NotTo(BeZero())
		Consistently(errC).ShouldNot(Receive(HaveOccurred()))
	})

	AfterAll(func() {
		Expect(environment.Stop()).To(Succeed())
	})
	//#endregion

	//#region Happy Path Tests

	events := make(chan *management.WatchEvent, 1000)
	It("should handle watching create and delete events", func() {
		stream, err := client.WatchClusters(context.Background(), &management.WatchClustersRequest{
			KnownClusters: &core.ReferenceList{},
		})
		Expect(err).NotTo(HaveOccurred())
		go func() {
			defer close(events)
			for {
				event, err := stream.Recv()
				if err != nil {
					return
				}
				events <- event
			}
		}()
	})

	It("can get information about a specific cluster", func() {
		clusterInfo, err := client.GetCluster(context.Background(), &core.Reference{
			Id: "test-cluster-id",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(clusterInfo.Id).To(Equal("test-cluster-id"))
		Expect(clusterInfo.GetLabels()).To(BeNil())
	})

	It("can edit the label a cluster is using", func() {
		_, err := client.EditCluster(context.Background(), &management.EditClusterRequest{
			Cluster: &core.Reference{
				Id: "test-cluster-id",
			},
			Labels: map[string]string{
				"i": "999",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		clusterInfo, err := client.GetCluster(context.Background(), &core.Reference{
			Id: "test-cluster-id",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(clusterInfo.Id).To(Equal("test-cluster-id"))
		Expect(clusterInfo.GetLabels()).To(HaveKeyWithValue("i", "999"))
	})

	var fingerprint2 string
	It("can list all clusters using the same label", func() {
		token2, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint2 = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())

		_, errC := environment.StartAgent("test-cluster-id-2", token2, []string{fingerprint2})
		Consistently(errC).ShouldNot(Receive())

		Eventually(events).Should(Receive(WithTransform(func(event *management.WatchEvent) string {
			return event.Cluster.Id
		}, Equal("test-cluster-id-2"))))

		_, err = client.EditCluster(context.Background(), &management.EditClusterRequest{
			Cluster: &core.Reference{
				Id: "test-cluster-id-2",
			},
			Labels: map[string]string{
				"i": "999",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		clusterInfo, err := client.ListClusters(context.Background(), &management.ListClustersRequest{
			MatchLabels: &core.LabelSelector{
				MatchLabels: map[string]string{
					"i": "999",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(clusterInfo.GetItems()).NotTo(BeNil())
	})

	It("can delete individual clusters", func() {
		_, errG1 := client.GetCluster(context.Background(), &core.Reference{
			Id: "test-cluster-id",
		})
		Expect(errG1).NotTo(HaveOccurred())

		_, errG2 := client.GetCluster(context.Background(), &core.Reference{
			Id: "test-cluster-id-2",
		})
		Expect(errG2).NotTo(HaveOccurred())

		_, errD := client.DeleteCluster(context.Background(), &core.Reference{
			Id: "test-cluster-id",
		})
		Expect(errD).NotTo(HaveOccurred())

		_, err := client.GetCluster(context.Background(), &core.Reference{
			Id: "test-cluster-id",
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Convert(err).Code()).To(Equal(codes.NotFound))

		clusterInfo, err := client.GetCluster(context.Background(), &core.Reference{
			Id: "test-cluster-id-2",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(clusterInfo.Id).To(Equal("test-cluster-id-2"))
	})

	//#endregion

	//#region Edge Case Tests

	It("cannot get information about a specific cluster without providing a valid ID", func() {
		_, err := client.GetCluster(context.Background(), &core.Reference{
			Id: uuid.NewString(),
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Convert(err).Code()).To(Equal(codes.NotFound))
	})

	It("cannot get information about a specific cluster without providing an ID", func() {
		_, err := client.GetCluster(context.Background(), &core.Reference{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))
	})

	It("cannot edit the label a cluster is using without providing a valid ID", func() {
		_, err := client.EditCluster(context.Background(), &management.EditClusterRequest{
			Cluster: &core.Reference{
				Id: uuid.NewString(),
			},
			Labels: map[string]string{
				"i": "999",
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get cluster: not found"))
	})

	It("cannot edit the label a cluster is using without providing an ID", func() {
		_, err := client.EditCluster(context.Background(), &management.EditClusterRequest{
			Cluster: &core.Reference{},
			Labels: map[string]string{
				"i": "999",
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))
	})

	It("cannot edit the label a cluster is using without providing Cluster information", func() {
		token, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		clusterName := uuid.NewString()
		_, errC := environment.StartAgent(clusterName, token, []string{fingerprint})
		Consistently(errC).ShouldNot(Receive())

		Eventually(func() error {
			_, err := client.GetCluster(context.Background(), &core.Reference{
				Id: clusterName,
			})
			return err
		}).Should(Succeed())

		_, err = client.EditCluster(context.Background(), &management.EditClusterRequest{
			Labels: map[string]string{
				"i": "999",
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: cluster"))
	})

	It("cannot list clusters by label without providing a valid label", func() {
		clusterList, err := client.ListClusters(context.Background(), &management.ListClustersRequest{
			MatchLabels: &core.LabelSelector{
				MatchLabels: map[string]string{
					"i": "99",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(clusterList.GetItems()).To(HaveLen(0))
	})

	When("editing a cluster without providing label information", func() {
		It("can remove labels from a cluster", func() {
			token, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())

			clusterName := uuid.NewString()
			_, errC := environment.StartAgent(clusterName, token, []string{fingerprint})
			Consistently(errC).ShouldNot(Receive())

			Eventually(func() error {
				_, err := client.GetCluster(context.Background(), &core.Reference{
					Id: clusterName,
				})
				return err
			}).Should(Succeed())

			_, errE := client.EditCluster(context.Background(), &management.EditClusterRequest{
				Cluster: &core.Reference{
					Id: clusterName,
				},
				Labels: map[string]string{
					"i": "999",
				},
			})
			Expect(errE).NotTo(HaveOccurred())

			_, err = client.EditCluster(context.Background(), &management.EditClusterRequest{
				Cluster: &core.Reference{
					Id: clusterName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			clusterInfo, err := client.GetCluster(context.Background(), &core.Reference{
				Id: clusterName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterInfo.GetLabels()).To(BeEmpty())
		})
	})

	It("cannot delete individual clusters without providing a valid ID", func() {
		_, err := client.DeleteCluster(context.Background(), &core.Reference{
			Id: uuid.NewString(),
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("cannot delete individual clusters without providing an ID", func() {
		_, err := client.DeleteCluster(context.Background(), &core.Reference{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))
	})

	//#endregion
})
