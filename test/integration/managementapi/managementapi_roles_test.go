package integration_test

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/test"
	"google.golang.org/protobuf/types/known/emptypb"
)

//#region Test Setup
var _ = Describe("Management API Roles Management Tests", Ordered, func() {
	var environment *test.Environment
	var client management.ManagementClient
	BeforeAll(func() {
		fmt.Println("Starting test environment")
		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())
	})

	AfterAll(func() {
		fmt.Println("Stopping test environment")
		Expect(environment.Stop()).To(Succeed())
	})

	//#endregion

	//#region Happy Path Tests

	When("creating a new role", func() {

		It("can get information about all roles", func() {
			var err error
			_, err = client.CreateRole(context.Background(), &core.Role{
				Name:       "test-role",
				ClusterIDs: []string{"test-cluster"},
				MatchLabels: &core.LabelSelector{
					MatchLabels: map[string]string{"test-label": "test-value"},
				},
			},
			)
			Expect(err).NotTo(HaveOccurred())

			roleInfo, err := client.GetRole(context.Background(), &core.Reference{
				Name: "test-role",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(roleInfo.Name).To(Equal("test-role"))
			Expect(roleInfo.ClusterIDs).To(Equal([]string{"test-cluster"}))
			Expect(roleInfo.MatchLabels).To(Equal(&core.LabelSelector{
				MatchLabels: map[string]string{"test-label": "test-value"},
			}))
		})
	})

	It("can list all roles", func() {
		role, err := client.ListRoles(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		roleList := role.Items
		Expect(roleList).To(HaveLen(1))
		for _, roleItem := range roleList {
			Expect(roleItem.Name).To(Equal("test-role"))
			Expect(roleItem.ClusterIDs).To(Equal([]string{"test-cluster"}))
			Expect(roleItem.MatchLabels).To(Equal(&core.LabelSelector{
				MatchLabels: map[string]string{"test-label": "test-value"},
			}))
		}
	})

	It("can delete an existing role", func() {
		_, err := client.DeleteRole(context.Background(), &core.Reference{
			Name: "test-role",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetRole(context.Background(), &core.Reference{
			Name: "test-role",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get role: not found"))
	})

	//#endregion

	//#region Edge Case Tests

	//#endregion

})
