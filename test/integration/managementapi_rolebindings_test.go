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
		client = environment.NewManagementClient()

		_, err := client.CreateRole(context.Background(), &corev1.Role{
			Id: "test-role",
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{"test": "test"},
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		ExpectGracefulExamplePluginShutdown(environment)
	})
	//#endregion

	//#region Happy Path Tests

	var err error
	When("creating a new rolebinding", func() {

		It("can get information about all rolebindings", func() {
			_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
				Id:       "test-rolebinding1",
				RoleId:   "test-role",
				Subjects: []string{"test-subject"},
			})
			Expect(err).NotTo(HaveOccurred())

			rolebindingInfo, err := client.GetRoleBinding(context.Background(), &corev1.Reference{
				Id: "test-rolebinding1",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(rolebindingInfo.Id).To(Equal("test-rolebinding1"))
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
			Expect(rolebindingItem.Id).To(Equal("test-rolebinding1"))
			Expect(rolebindingItem.RoleId).To(Equal("test-role"))
			Expect(rolebindingItem.Subjects).To(ContainElement("test-subject"))
		}
	})

	It("can update an existing role binding", func() {
		_, err := client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding1",
		})
		Expect(err).NotTo(HaveOccurred())

		rb := &corev1.RoleBinding{
			Id:       "test-rolebinding1",
			RoleId:   "updated-test-role",
			Subjects: []string{"updated-test-subject"},
		}
		_, err = client.UpdateRoleBinding(context.Background(), rb)
		Expect(err).NotTo(HaveOccurred())

		rbInfo, err := client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding1",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(rbInfo.Id).To(Equal("test-rolebinding1"))
		Expect(rbInfo.RoleId).To(Equal("updated-test-role"))
		Expect(rbInfo.Subjects).To(ContainElement("updated-test-subject"))
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
			Id:       "test-rolebinding2",
			Subjects: []string{"test-subject"},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: roleId"))

		_, err = client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding2",
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})

	It("can create rolebindings without a valid RoleId", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			RoleId:   uuid.NewString(),
			Id:       "test-rolebinding2",
			Subjects: []string{"test-subject"},
		})
		Expect(err).NotTo(HaveOccurred())

		rbInfo, err := client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding2",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(rbInfo.Taints).To(ContainElement("role not found"))
	})

	It("cannot create rolebindings without an Id", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing required field: id"))
	})

	It("cannot create rolebindings without a subject", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:     "test-rolebinding3",
			RoleId: "test-role",
		})
		Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument, ContainSubstring("missing required field: subjects")))

		_, err = client.GetRoleBinding(context.Background(), &corev1.Reference{
			Id: "test-rolebinding3",
		})
		Expect(err).To(testutil.MatchStatusCode(codes.NotFound))
	})

	It("cannot delete a rolebinding without specifying an Id", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:       "test-rolebinding5",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
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
			Id:       "test-rolebinding6",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
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
			Id:       "test-rolebinding7",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:       "test-rolebinding7",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
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
			Id:     "does-not-exist",
			RoleId: "test-role",
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})

	It("cannot update read only role binding taints", func() {
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:       "test-rolebinding8",
			RoleId:   "test-role",
			Subjects: []string{"test-subject"},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.UpdateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:     "test-rolebinding8",
			Taints: []string{"modified-taint"},
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
	})
	//#endregion

})
