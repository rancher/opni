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
var _ = Describe("Management API Rolebinding Management Tests", Ordered, func() {
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

	When("creating a new rolebinding", func() {

		It("can get information about all rolebindings", func() {
			var err error
			_, err = client.CreateRole(context.Background(), &core.Role{
				Name: "test-role",
			},
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
				Name:     "test-rolebinding",
				RoleName: "test-role",
				Subjects: []string{"test-subject"},
				Taints:   []string{"test-taint"},
			})

			Expect(err).NotTo(HaveOccurred())

			rolebindingInfo, err := client.GetRoleBinding(context.Background(), &core.Reference{
				Name: "test-rolebinding",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(rolebindingInfo.Name).To(Equal("test-rolebinding"))
			Expect(rolebindingInfo.RoleName).To(Equal("test-role"))
			Expect(rolebindingInfo.Subjects).To(Equal([]string{"test-subject"}))
			Expect(rolebindingInfo.Taints).To(Equal([]string{"test-taint"}))
		})
	})

	It("can list all rolebindings", func() {
		rolebinding, err := client.ListRoleBindings(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		rolebindingList := rolebinding.Items
		Expect(rolebindingList).To(HaveLen(1))
		for _, rolebindingItem := range rolebindingList {
			Expect(rolebindingItem.Name).To(Equal("test-rolebinding"))
			Expect(rolebindingItem.RoleName).To(Equal("test-role"))
			Expect(rolebindingItem.Subjects).To(ContainElement("test-subject"))
			Expect(rolebindingItem.Taints).To(ContainElement("test-taint"))
		}
	})

	It("can delete an existing rolebinding", func() {

		_, err := client.DeleteRoleBinding(context.Background(), &core.Reference{
			Name: "test-rolebinding",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetRoleBinding(context.Background(), &core.Reference{
			Name: "test-rolebinding",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get role binding: not found"))
	})

	//#endregion

	//#region Edge Case Tests

	//#endregion

})
