package conformance

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/future"
)

func RBACStoreTestSuite[T storage.RBACStore](
	tsF future.Future[T],
) func() {
	return func() {
		var ts T
		BeforeAll(func() {
			ts = tsF.Get()
		})
		Context("Roles", func() {
			It("should initially have no roles", func() {
				roles, err := ts.ListRoles(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(roles.Items).To(BeEmpty())
			})
			When("creating a role", func() {
				It("should be retrievable", func() {
					role := &corev1.Role{
						Id: "foo",
					}
					Eventually(func() error {
						return ts.CreateRole(context.Background(), role)
					}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

					role, err := ts.GetRole(context.Background(), role.Reference())
					Expect(err).NotTo(HaveOccurred())
					Expect(role).NotTo(BeNil())
					Expect(role.Id).To(Equal("foo"))
				})
				It("should appear in the list of roles", func() {
					roles, err := ts.ListRoles(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(roles.Items).To(HaveLen(1))
					Expect(roles.Items[0].GetId()).To(Equal("foo"))
				})
			})
			It("should delete roles", func() {
				all, err := ts.ListRoles(context.Background())
				Expect(err).NotTo(HaveOccurred())

				for _, role := range all.Items {
					err := ts.DeleteRole(context.Background(), role.Reference())
					Expect(err).NotTo(HaveOccurred())
				}

				all, err = ts.ListRoles(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(all.Items).To(BeEmpty())
			})
		})
		Context("Role Bindings", func() {
			It("should initially have no role bindings", func() {
				rbs, err := ts.ListRoleBindings(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(rbs.Items).To(BeEmpty())
			})
			When("creating a role binding", func() {
				It("should be retrievable", func() {
					rb := &corev1.RoleBinding{
						Id: "foo",
					}
					Eventually(func() error {
						return ts.CreateRoleBinding(context.Background(), rb)
					}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

					rb, err := ts.GetRoleBinding(context.Background(), rb.Reference())
					Expect(err).NotTo(HaveOccurred())
					Expect(rb).NotTo(BeNil())
					Expect(rb.Id).To(Equal("foo"))
				})
				It("should appear in the list of role bindings", func() {
					rbs, err := ts.ListRoleBindings(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(rbs.Items).To(HaveLen(1))
					Expect(rbs.Items[0].GetId()).To(Equal("foo"))
				})
			})
			It("should delete role bindings", func() {
				all, err := ts.ListRoleBindings(context.Background())
				Expect(err).NotTo(HaveOccurred())

				for _, rb := range all.Items {
					err := ts.DeleteRoleBinding(context.Background(), rb.Reference())
					Expect(err).NotTo(HaveOccurred())
				}

				all, err = ts.ListRoleBindings(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(all.Items).To(BeEmpty())
			})
		})
	}
}
