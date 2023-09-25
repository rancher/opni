package management_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("RBAC", Ordered, Label("unit"), func() {
	var tv *testVars
	BeforeAll(setupManagementServer(&tv, plugins.NoopLoader))

	It("should initially have no RBAC objects", func() {
		rbs, err := tv.client.ListRoleBindings(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(rbs.Items).To(BeEmpty())
	})
	It("should create role bindings", func() {
		for i := 0; i < 100; i++ {
			rb := &corev1.RoleBinding{
				Id:      fmt.Sprintf("rb-%d", i),
				RoleIds: []string{fmt.Sprintf("role-%d", i)},
				Subject: fmt.Sprintf("user-%d", i),
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

	It("should delete role bindings", func() {
		for i := 0; i < 100; i++ {
			rb := &corev1.RoleBinding{
				Id: fmt.Sprintf("rb-%d", i),
			}
			_, err := tv.client.DeleteRoleBinding(context.Background(), rb.Reference())
			Expect(err).NotTo(HaveOccurred())

			_, err = tv.client.GetRoleBinding(context.Background(), rb.Reference())
			Expect(util.StatusCode(err)).To(Equal(codes.NotFound))

			rbs, err := tv.client.ListRoleBindings(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rbs.Items).To(HaveLen(100 - i - 1))
		}
	})

	Context("error handling", func() {
		When("creating a rolebinding with taints", func() {
			It("should error indicating the field is read-only", func() {
				rb := &corev1.RoleBinding{
					Id:      "rb-1",
					RoleIds: []string{"role-1"},
					Subject: "user-1",
					Taints:  []string{"foo"},
				}
				_, err := tv.client.CreateRoleBinding(context.Background(), rb)
				Expect(err).To(HaveOccurred())
				Expect(status.Convert(err).Message()).To(Equal(validation.ErrReadOnlyField.Error()))
			})
		})
	})
})
