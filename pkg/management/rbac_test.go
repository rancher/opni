package management_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/core"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("RBAC", Ordered, Label(test.Slow), func() {
	var tv *testVars
	BeforeAll(setupManagementServer(&tv))

	It("should initially have no RBAC objects", func() {
		roles, err := tv.client.ListRoles(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(roles.Items).To(BeEmpty())
		rbs, err := tv.client.ListRoleBindings(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(rbs.Items).To(BeEmpty())
	})
	It("should create roles", func() {
		for i := 0; i < 100; i++ {
			role := &core.Role{
				Id:         fmt.Sprintf("role-%d", i),
				ClusterIDs: []string{fmt.Sprintf("cluster-%d", i)},
			}
			_, err := tv.client.CreateRole(context.Background(), role)
			Expect(err).NotTo(HaveOccurred())

			created, err := tv.client.GetRole(context.Background(), role.Reference())
			Expect(err).NotTo(HaveOccurred())
			Expect(created.Id).To(Equal(role.Id))
		}

		roles, err := tv.client.ListRoles(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(roles.Items).To(HaveLen(100))
	})
	It("should create role bindings", func() {
		for i := 0; i < 100; i++ {
			rb := &core.RoleBinding{
				Id:       fmt.Sprintf("rb-%d", i),
				RoleId:   fmt.Sprintf("role-%d", i),
				Subjects: []string{fmt.Sprintf("user-%d", i)},
			}
			_, err := tv.client.CreateRoleBinding(context.Background(), rb)
			Expect(err).NotTo(HaveOccurred())

			created, err := tv.client.GetRoleBinding(context.Background(), rb.Reference())
			Expect(err).NotTo(HaveOccurred())
			Expect(created.Id).To(Equal(rb.Id))
		}

		rbs, err := tv.client.ListRoleBindings(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(rbs.Items).To(HaveLen(100))
	})

	It("should compute subject access", func() {
		for i := 0; i < 100; i++ {
			refList, err := tv.client.SubjectAccess(context.Background(), &core.SubjectAccessRequest{
				Subject: fmt.Sprintf("user-%d", i),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(refList.Items).To(HaveLen(1))
			Expect(refList.Items[0].Id).To(Equal(fmt.Sprintf("cluster-%d", i)))
		}
	})

	It("should delete roles", func() {
		for i := 0; i < 100; i++ {
			role := &core.Role{
				Id: fmt.Sprintf("role-%d", i),
			}
			_, err := tv.client.DeleteRole(context.Background(), role.Reference())
			Expect(err).NotTo(HaveOccurred())

			_, err = tv.client.GetRole(context.Background(), role.Reference())
			Expect(status.Code(err)).To(Equal(codes.NotFound))

			roles, err := tv.client.ListRoles(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(roles.Items).To(HaveLen(100 - i - 1))
		}
	})

	It("should delete role bindings", func() {
		for i := 0; i < 100; i++ {
			rb := &core.RoleBinding{
				Id: fmt.Sprintf("rb-%d", i),
			}
			_, err := tv.client.DeleteRoleBinding(context.Background(), rb.Reference())
			Expect(err).NotTo(HaveOccurred())

			_, err = tv.client.GetRoleBinding(context.Background(), rb.Reference())
			Expect(status.Code(err)).To(Equal(codes.NotFound))

			rbs, err := tv.client.ListRoleBindings(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rbs.Items).To(HaveLen(100 - i - 1))
		}
	})

	Context("error handling", func() {
		When("creating a rolebinding with taints", func() {
			It("should error indicating the field is read-only", func() {
				rb := &core.RoleBinding{
					Id:       "rb-1",
					RoleId:   "role-1",
					Subjects: []string{"user-1"},
					Taints:   []string{"foo"},
				}
				_, err := tv.client.CreateRoleBinding(context.Background(), rb)
				Expect(err).To(HaveOccurred())
				Expect(status.Convert(err).Message()).To(Equal(validation.ErrReadOnlyField.Error()))
			})
		})
	})
})
