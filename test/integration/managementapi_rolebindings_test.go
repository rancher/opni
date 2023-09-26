package integration_test

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testutil"
)

// #region Test Setup
var _ = Describe("Management API Rolebinding Management Tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var client managementv1.ManagementClient
	BeforeAll(func() {
		environment = &test.Environment{}
		Expect(environment.Start()).To(Succeed())
		DeferCleanup(environment.Stop)
		client = environment.NewManagementClient()
	})

	//#endregion

	//#region Happy Path Tests

	var err error
	When("creating a new rolebinding", func() {

		It("can get information about all rolebindings", func() {
			_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
				Id:      "test-rolebinding1",
				RoleIds: []string{"test-role"},
				Subject: "test-subject",
			})
			Expect(err).NotTo(HaveOccurred())

			rolebindingInfo, err := client.GetRoleBinding(context.Background(), &corev1.Reference{
				Id: "test-rolebinding1",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(rolebindingInfo.Id).To(Equal("test-rolebinding1"))
			Expect(rolebindingInfo.RoleIds).To(Equal([]string{"test-role"}))
			Expect(rolebindingInfo.Subject).To(Equal("test-subject"))
		})
	})

	It("can list all rolebindings", func() {
		rolebinding, err := client.ListRoleBindings(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		rolebindingList := rolebinding.Items
		Expect(rolebindingList).To(HaveLen(1))
		for _, rolebindingItem := range rolebindingList {
			Expect(rolebindingItem.Id).To(Equal("test-rolebinding1"))
			Expect(rolebindingItem.RoleIds).To(ContainElement("test-role"))
			Expect(rolebindingItem.Subject).To(Equal("test-subject"))
		}
	})

	It("can update an existing role binding", func() {
		_, err := client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding1",
		})
		Expect(err).NotTo(HaveOccurred())

		rb := &corev1.RoleBinding{
			Id:      "test-rolebinding1",
			RoleIds: []string{"updated-test-role"},
			Subject: "updated-test-subject",
		}
		_, err = client.UpdateRoleBinding(context.Background(), rb)
		Expect(err).NotTo(HaveOccurred())

		rbInfo, err := client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding1",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(rbInfo.Id).To(Equal("test-rolebinding1"))
		Expect(rbInfo.RoleIds).To(ContainElement("updated-test-role"))
		Expect(rbInfo.Subject).To(Equal("updated-test-subject"))
	})

	It("can delete an existing rolebinding", func() {
		_, err = client.DeleteRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding1",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding1",
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})

	//#endregion

	//#region Edge Case Tests

	It("cannot create rolebindings without a RoleID", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:      "test-rolebinding2",
			Subject: "test-subject",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: roleIds"))

		_, err = client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding2",
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})

	It("can create rolebindings without a valid RoleId", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			RoleIds: []string{uuid.NewString()},
			Id:      "test-rolebinding2",
			Subject: "test-subject",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err := client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding2",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot create rolebindings without an Id", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			RoleIds: []string{"test-role"},
			Subject: "test-subject",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))
	})

	It("cannot create rolebindings without a subject", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:      "test-rolebinding3",
			RoleIds: []string{"test-role"},
		})
		Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument, ContainSubstring("missing required field: subject")))

		_, err = client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding3",
		})
		Expect(err).To(testutil.MatchStatusCode(codes.NotFound))
	})

	It("cannot delete a rolebinding without specifying an Id", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:      "test-rolebinding5",
			RoleIds: []string{"test-role"},
			Subject: "test-subject",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.DeleteRoleBinding(context.Background(), &corev1.Reference{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))

		_, err = client.DeleteRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding5",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot delete a rolebinding without specifying a valid Id", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:      "test-rolebinding6",
			RoleIds: []string{"test-role"},
			Subject: "test-subject",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.DeleteRoleBinding(context.Background(), &corev1.Reference{
			Id: uuid.NewString(),
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))

		_, err = client.DeleteRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding6",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot create rolebindings with identical Ids", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:      "test-rolebinding7",
			RoleIds: []string{"test-role"},
			Subject: "test-subject",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:      "test-rolebinding7",
			RoleIds: []string{"test-role"},
			Subject: "test-subject",
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.AlreadyExists))

		_, err = client.DeleteRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding7",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot update a non existent role binding", func() {
		_, err = client.UpdateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:      "does-not-exist",
			RoleIds: []string{"test-role"},
			Subject: "test-subject",
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})

	It("cannot update read only role binding taints", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:      "test-rolebinding8",
			RoleIds: []string{"test-role"},
			Subject: "test-subject",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.UpdateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:      "test-rolebinding8",
			RoleIds: []string{"test-role"},
			Subject: "test-subject",
			Taints:  []string{"modified-taint"},
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
	})
	//#endregion

})
