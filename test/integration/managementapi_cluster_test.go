package integration_test

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	_ "github.com/rancher/opni/plugins/example/test"
)

// #region Test Setup
var _ = Describe("Management API Cluster Management Tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var client managementv1.ManagementClient
	var fingerprint string
	BeforeAll(func() {
		environment = &test.Environment{}
		Expect(environment.Start()).To(Succeed())
		DeferCleanup(environment.Stop)

		client = environment.NewManagementClient()

		token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())

		_, errC := environment.StartAgent("test-cluster-id", token, []string{fingerprint})
		Eventually(errC).Should(Receive(BeNil()))
	})

	//#endregion

	//#region Happy Path Tests

	It("can get information about a specific cluster", func() {
		clusterInfo, err := client.GetCluster(context.Background(), &corev1.Reference{
			Id: "test-cluster-id",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(clusterInfo.Id).To(Equal("test-cluster-id"))
	})

	It("can edit the label a cluster is using", func() {
		clusterInfo, err := client.GetCluster(context.Background(), &corev1.Reference{
			Id: "test-cluster-id",
		})
		Expect(err).NotTo(HaveOccurred())
		labels := clusterInfo.GetLabels()
		labels["i"] = "999"
		_, err = client.EditCluster(context.Background(), &managementv1.EditClusterRequest{
			Cluster: &corev1.Reference{
				Id: "test-cluster-id",
			},
			Labels: labels,
		})
		Expect(err).NotTo(HaveOccurred())

		clusterInfo, err = client.GetCluster(context.Background(), &corev1.Reference{
			Id: "test-cluster-id",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(clusterInfo.Id).To(Equal("test-cluster-id"))
		Expect(clusterInfo.GetLabels()).To(HaveKeyWithValue("i", "999"))
	})

	It("can list all clusters using the same label", func() {
		err := environment.BootstrapNewAgent("test-cluster-id-2")
		Expect(err).NotTo(HaveOccurred())

		clusterInfo, err := client.GetCluster(context.Background(), &corev1.Reference{
			Id: "test-cluster-id",
		})
		Expect(err).NotTo(HaveOccurred())
		labels := clusterInfo.GetLabels()
		labels["i"] = "999"
		_, err = client.EditCluster(context.Background(), &managementv1.EditClusterRequest{
			Cluster: &corev1.Reference{
				Id: "test-cluster-id-2",
			},
			Labels: labels,
		})
		Expect(err).NotTo(HaveOccurred())

		clusterInfoList, err := client.ListClusters(context.Background(), &managementv1.ListClustersRequest{
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{
					"i": "999",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(clusterInfoList.GetItems()).To(HaveLen(2))
	})

	When("a cluster has installed capabilities", func() {
		It("should prevent the cluster from being deleted", func() {
			_, err := client.InstallCapability(context.Background(), &capabilityv1.InstallRequest{
				Capability:     &corev1.Reference{Id: wellknown.CapabilityExample},
				Agent:          &corev1.Reference{Id: "test-cluster-id"},
				IgnoreWarnings: true,
			})
			Expect(err).NotTo(HaveOccurred())
			_, errG1 := client.GetCluster(context.Background(), &corev1.Reference{
				Id: "test-cluster-id",
			})
			Expect(errG1).NotTo(HaveOccurred())

			_, errD := client.DeleteCluster(context.Background(), &corev1.Reference{
				Id: "test-cluster-id",
			})
			Expect(util.StatusCode(errD)).To(Equal(codes.FailedPrecondition))
		})
		It("should allow uninstalling capabilities", func() {
			_, err := client.UninstallCapability(context.Background(), &capabilityv1.UninstallRequest{
				Capability: &corev1.Reference{Id: wellknown.CapabilityExample},
				Agent:      &corev1.Reference{Id: "test-cluster-id"},
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				status, err := client.CapabilityUninstallStatus(context.Background(), &capabilityv1.UninstallStatusRequest{
					Capability: &corev1.Reference{Id: wellknown.CapabilityExample},
					Agent:      &corev1.Reference{Id: "test-cluster-id"},
				})
				if err != nil {
					return err
				}
				if status.State == task.StateCompleted {
					return nil
				}
				return fmt.Errorf("waiting; status: %+v", status)
			}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

			status, err := client.CapabilityUninstallStatus(context.Background(), &capabilityv1.UninstallStatusRequest{
				Capability: &corev1.Reference{Id: wellknown.CapabilityExample},
				Agent:      &corev1.Reference{Id: "test-cluster-id"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(status).NotTo(BeNil())
			Expect(status.State).To(Equal(task.StateCompleted))
			AddReportEntry("logs", status.Logs)
		})
		It("should allow the cluster to be deleted once all capabilities are uninstalled", func() {
			_, errG1 := client.GetCluster(context.Background(), &corev1.Reference{
				Id: "test-cluster-id",
			})
			Expect(errG1).NotTo(HaveOccurred())

			_, errD := client.DeleteCluster(context.Background(), &corev1.Reference{
				Id: "test-cluster-id",
			})
			Expect(errD).NotTo(HaveOccurred())
		})
	})

	//#endregion

	//#region Edge Case Tests

	It("cannot get information about a specific cluster without providing a valid ID", func() {
		_, err := client.GetCluster(context.Background(), &corev1.Reference{
			Id: uuid.NewString(),
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Convert(err).Code()).To(Equal(codes.NotFound))
	})

	It("cannot get information about a specific cluster without providing an ID", func() {
		_, err := client.GetCluster(context.Background(), &corev1.Reference{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))
	})

	It("cannot edit the label a cluster is using without providing a valid ID", func() {
		_, err := client.EditCluster(context.Background(), &managementv1.EditClusterRequest{
			Cluster: &corev1.Reference{
				Id: uuid.NewString(),
			},
			Labels: map[string]string{
				"i": "999",
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})

	It("cannot edit the label a cluster is using without providing an ID", func() {
		_, err := client.EditCluster(context.Background(), &managementv1.EditClusterRequest{
			Cluster: &corev1.Reference{},
			Labels: map[string]string{
				"i": "999",
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))
	})

	It("cannot edit the label a cluster is using without providing Cluster information", func() {
		token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		clusterName := uuid.NewString()
		_, errC := environment.StartAgent(clusterName, token, []string{fingerprint})
		Eventually(errC).Should(Receive(BeNil()))

		Eventually(func() error {
			_, err := client.GetCluster(context.Background(), &corev1.Reference{
				Id: clusterName,
			})
			return err
		}).Should(Succeed())

		_, err = client.EditCluster(context.Background(), &managementv1.EditClusterRequest{
			Labels: map[string]string{
				"i": "999",
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: cluster"))
	})

	It("cannot list clusters by label without providing a valid label", func() {
		clusterList, err := client.ListClusters(context.Background(), &managementv1.ListClustersRequest{
			MatchLabels: &corev1.LabelSelector{
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
			clusterName := uuid.NewString()
			err := environment.BootstrapNewAgent(clusterName)
			Expect(err).NotTo(HaveOccurred())

			var labels map[string]string
			Eventually(func() error {
				info, err := client.GetCluster(context.Background(), &corev1.Reference{
					Id: clusterName,
				})
				if err != nil {
					return err
				}
				labels = info.GetLabels()
				return nil
			}).Should(Succeed())

			labels["i"] = "999"
			_, errE := client.EditCluster(context.Background(), &managementv1.EditClusterRequest{
				Cluster: &corev1.Reference{
					Id: clusterName,
				},
				Labels: labels,
			})
			Expect(errE).NotTo(HaveOccurred())

			delete(labels, "i")
			_, err = client.EditCluster(context.Background(), &managementv1.EditClusterRequest{
				Cluster: &corev1.Reference{
					Id: clusterName,
				},
				Labels: labels,
			})
			Expect(err).NotTo(HaveOccurred())

			clusterInfo, err := client.GetCluster(context.Background(), &corev1.Reference{
				Id: clusterName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterInfo.GetLabels()).NotTo(HaveKey("i"))
		})
	})

	It("cannot delete individual clusters without providing a valid ID", func() {
		_, err := client.DeleteCluster(context.Background(), &corev1.Reference{
			Id: uuid.NewString(),
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("cannot delete individual clusters without providing an ID", func() {
		_, err := client.DeleteCluster(context.Background(), &corev1.Reference{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))
	})

	//#endregion
})
