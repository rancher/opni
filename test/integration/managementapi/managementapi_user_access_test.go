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
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

//#region Test Setup
var _ = FDescribe("Management API User/Subject Access Management Tests", Ordered, func() {
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
		Consistently(errC).ShouldNot(Receive())
	})

	AfterAll(func() {
		Expect(environment.Stop()).To(Succeed())
	})

	//#endregion

	//#region Happy Path Tests

	It("can return a list of all Cluster IDs that a specific User (Subject) can access", func() {
		_, err := client.CreateRole(context.Background(), &core.Role{
			Id:         "test-role",
			ClusterIDs: []string{"test-cluster-id"},
		},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
			Id:       "test-rolebinding",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
		})
		Expect(err).NotTo(HaveOccurred())

		accessList, err := client.SubjectAccess(context.Background(), &core.SubjectAccessRequest{
			Subject: "test-subject",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(accessList.Items).To(HaveLen(1))
		Expect(accessList.Items[0].Id).To(Equal("test-cluster-id"))
	})

	It("can return a list of all Cluster IDs that a specific User (Subject) can access via labels", func() {
		_, err := client.CreateRole(context.Background(), &core.Role{
			Id: "test-role",
			MatchLabels: &core.LabelSelector{
				MatchLabels: map[string]string{"i": "999"},
			},
		},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
			Id:       "test-rolebinding",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
		})
		Expect(err).NotTo(HaveOccurred())

		token, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		clusterNameList := make([]string, 10)
		for i := 0; i < 10; i++ {
			clusterName := "test-cluster-id-" + uuid.New().String()
			clusterNameList = append(clusterNameList, clusterName)

			_, errC := environment.StartAgent(clusterName, token, []string{fingerprint})
			Consistently(errC).ShouldNot(Receive())

			_, err := client.EditCluster(context.Background(), &management.EditClusterRequest{
				Cluster: &core.Reference{
					Id: clusterName,
				},
				Labels: map[string]string{
					"i": "999",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		}

		accessList, err := client.SubjectAccess(context.Background(), &core.SubjectAccessRequest{
			Subject: "test-subject",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(accessList.Items).To(HaveLen(10))
		Expect(accessList.Items[0].Id).To(ContainSubstring(clusterNameList[0]))
		Expect(accessList.Items[1].Id).To(ContainSubstring(clusterNameList[1]))
		Expect(accessList.Items[2].Id).To(ContainSubstring(clusterNameList[2]))
		Expect(accessList.Items[3].Id).To(ContainSubstring(clusterNameList[3]))
		Expect(accessList.Items[4].Id).To(ContainSubstring(clusterNameList[4]))
		Expect(accessList.Items[5].Id).To(ContainSubstring(clusterNameList[5]))
		Expect(accessList.Items[6].Id).To(ContainSubstring(clusterNameList[6]))
		Expect(accessList.Items[7].Id).To(ContainSubstring(clusterNameList[7]))
		Expect(accessList.Items[8].Id).To(ContainSubstring(clusterNameList[8]))
		Expect(accessList.Items[9].Id).To(ContainSubstring(clusterNameList[9]))
	})

	//#endregion

	//#region Edge Case Tests

	//TODO: Add User Access Edge Case Tests

	//#endregion
})
