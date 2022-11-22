package conformance_storage

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
)

func BuildRBACStoreTestSuite[T storage.RBACStore](pt *T) bool {
	return Describe("RBAC Store", Ordered, Label("integration", "slow"), rbacStoreTestSuite(pt))
}

func rbacStoreTestSuite[T storage.RBACStore](pt *T) func() {
	return func() {
		var t T
		BeforeAll(func() {
			t = *pt
		})
		Context("Roles", func() {
			It("should initially have no roles", func() {
				roles, err := t.ListRoles(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(roles.Items).To(BeEmpty())
			})
			When("creating a role", func() {
				It("should be retrievable", func() {
					role := &corev1.Role{
						Id: "foo",
					}
					Eventually(func() error {
						return t.CreateRole(context.Background(), role)
					}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

					role, err := t.GetRole(context.Background(), role.Reference())
					Expect(err).NotTo(HaveOccurred())
					Expect(role).NotTo(BeNil())
					Expect(role.Id).To(Equal("foo"))
				})
				It("should appear in the list of roles", func() {
					roles, err := t.ListRoles(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(roles.Items).To(HaveLen(1))
					Expect(roles.Items[0].GetId()).To(Equal("foo"))
				})
			})
			It("should delete roles", func() {
				all, err := t.ListRoles(context.Background())
				Expect(err).NotTo(HaveOccurred())

				for _, role := range all.Items {
					err := t.DeleteRole(context.Background(), role.Reference())
					Expect(err).NotTo(HaveOccurred())
				}

				all, err = t.ListRoles(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(all.Items).To(BeEmpty())
			})
			It("should fail if the role already exists", func() {
				role := &corev1.Role{
					Id: "foo",
				}
				err := t.CreateRole(context.Background(), role)
				Expect(err).NotTo(HaveOccurred())

				err = t.CreateRole(context.Background(), role)
				Expect(err).To(MatchError(storage.ErrAlreadyExists))
			})
		})
		Context("Role Bindings", func() {
			It("should initially have no role bindings", func() {
				rbs, err := t.ListRoleBindings(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(rbs.Items).To(BeEmpty())
			})
			When("creating a role binding", func() {
				It("should be retrievable", func() {
					rb := &corev1.RoleBinding{
						Id: "foo",
					}
					Eventually(func() error {
						return t.CreateRoleBinding(context.Background(), rb)
					}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

					rb, err := t.GetRoleBinding(context.Background(), rb.Reference())
					Expect(err).NotTo(HaveOccurred())
					Expect(rb).NotTo(BeNil())
					Expect(rb.Id).To(Equal("foo"))
				})
				It("should appear in the list of role bindings", func() {
					rbs, err := t.ListRoleBindings(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(rbs.Items).To(HaveLen(1))
					Expect(rbs.Items[0].GetId()).To(Equal("foo"))
				})
			})
			It("should delete role bindings", func() {
				all, err := t.ListRoleBindings(context.Background())
				Expect(err).NotTo(HaveOccurred())

				for _, rb := range all.Items {
					err := t.DeleteRoleBinding(context.Background(), rb.Reference())
					Expect(err).NotTo(HaveOccurred())
				}

				all, err = t.ListRoleBindings(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(all.Items).To(BeEmpty())
			})
			It("should fail if the role binding already exists", func() {
				rb := &corev1.RoleBinding{
					Id: "foo",
				}
				err := t.CreateRoleBinding(context.Background(), rb)
				Expect(err).NotTo(HaveOccurred())

				err = t.CreateRoleBinding(context.Background(), rb)
				Expect(err).To(MatchError(storage.ErrAlreadyExists))
			})
		})
	}
}
