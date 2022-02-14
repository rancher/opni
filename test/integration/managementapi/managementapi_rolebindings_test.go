package integration_test

import (
	"context"
	"encoding/json"

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
		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())

		_, err := client.CreateRole(context.Background(), &core.Role{
			Id: "test-role",
		},
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		Expect(environment.Stop()).To(Succeed())
	})
	//#endregion

	//#region Happy Path Tests

	var err error
	When("creating a new rolebinding", func() {

		It("can get information about all rolebindings", func() {
			_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
				Id:       "test-rolebinding",
				RoleId:   "test-role",
				Subjects: []string{"test-subject"},
			})
			Expect(err).NotTo(HaveOccurred())

			rolebindingInfo, err := client.GetRoleBinding(context.Background(), &core.Reference{
				Id: "test-rolebinding",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(rolebindingInfo.Id).To(Equal("test-rolebinding"))
			Expect(rolebindingInfo.RoleId).To(Equal("test-role"))
			Expect(rolebindingInfo.Subjects).To(Equal([]string{"test-subject"}))
		})
	})

	It("can list all rolebindings", func() {
		rolebinding, err := client.ListRoleBindings(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		rolebindingList := rolebinding.Items
		Expect(rolebindingList).To(HaveLen(1))
		for _, rolebindingItem := range rolebindingList {
			Expect(rolebindingItem.Id).To(Equal("test-rolebinding"))
			Expect(rolebindingItem.RoleId).To(Equal("test-role"))
			Expect(rolebindingItem.Subjects).To(ContainElement("test-subject"))
		}
	})

	It("can delete an existing rolebinding", func() {
		_, err = client.DeleteRoleBinding(context.Background(), &core.Reference{
			Id: "test-rolebinding",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetRoleBinding(context.Background(), &core.Reference{
			Id: "test-rolebinding",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get role binding: not found"))
	})

	//#endregion

	//#region Edge Case Tests

	It("cannot create rolebindings without a role", func() {
		_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
			Id:       "test-rolebinding",
			Subjects: []string{"test-subject"},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: roleId"))

		_, err = client.GetRoleBinding(context.Background(), &core.Reference{
			Id: "test-rolebinding",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get role binding: not found"))
	})

	It("cannot create rolebindings without an Id", func() {
		_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))

		_, err = client.GetRoleBinding(context.Background(), &core.Reference{
			Id: "test-rolebinding",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get role binding: not found"))
	})

	It("can create and get rolebindings without a subject", func() {
		_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
			Id:     "test-rolebinding",
			RoleId: "test-role",
		})
		Expect(err).NotTo(HaveOccurred())

		rolebindingInfo, err := client.GetRoleBinding(context.Background(), &core.Reference{
			Id: "test-rolebinding",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(rolebindingInfo.Id).To(Equal("test-rolebinding"))
		Expect(rolebindingInfo.RoleId).To(Equal("test-role"))
		Expect(rolebindingInfo.Subjects).To(BeNil())
		Expect(rolebindingInfo.Taints).To(Equal([]string{"no subjects"}))

		_, err = client.DeleteRoleBinding(context.Background(), &core.Reference{
			Id: "test-rolebinding",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("can create and get rolebindings without a taint", func() {
		_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
			Id:       "test-rolebinding",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
		})
		Expect(err).NotTo(HaveOccurred())

		rolebindingInfo, err := client.GetRoleBinding(context.Background(), &core.Reference{
			Id: "test-rolebinding",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(rolebindingInfo.Id).To(Equal("test-rolebinding"))
		Expect(rolebindingInfo.RoleId).To(Equal("test-role"))
		Expect(rolebindingInfo.Subjects).To(ContainElement("test-subject"))
		Expect(rolebindingInfo.Taints).To(BeNil())

		_, err = client.DeleteRoleBinding(context.Background(), &core.Reference{
			Id: "test-rolebinding",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot delete a rolebinding without specifying an Id", func() {
		_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
			Id:       "test-rolebinding",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.DeleteRoleBinding(context.Background(), &core.Reference{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))

		_, err = client.DeleteRoleBinding(context.Background(), &core.Reference{
			Id: "test-rolebinding",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	//TODO: This can be unignored once this functionality is implemented
	XIt("cannot create rolebindings with identical Ids", func() {
		_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
			Id:       "test-rolebinding",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
			Id:       "test-rolebinding",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create rolebinding: already exists"))

		_, err = client.DeleteRoleBinding(context.Background(), &core.Reference{
			Id: "test-rolebinding",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	//#endregion

})
