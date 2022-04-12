package conformance

import (
	"context"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util"
)

func RBACStoreTestSuite[T storage.RBACStore](
	tsF *util.Future[T],
	errCtrlF *util.Future[ErrorController],
) func() {
	return func() {
		var ts T
		var errCtrl ErrorController
		BeforeAll(func() {
			ts = tsF.Get()
			errCtrl = errCtrlF.Get()
		})
		Context("Roles", func() {
			It("should initially have no roles", func() {
				roles, err := ts.ListRoles(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(roles.Items).To(BeEmpty())
			})
			When("creating a role", func() {
				It("should be retrievable", func() {
					role := &core.Role{
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
			Context("error handling", func() {
				It("should handle errors when creating a role", func() {
					errCtrl.EnableErrors()
					defer errCtrl.DisableErrors()
					Eventually(func() error {
						err := ts.CreateRole(context.Background(), &core.Role{
							Id: uuid.NewString(),
						})
						return err
					}).Should(HaveOccurred())

				})
				It("should handle errors when getting a role", func() {
					_, err := ts.GetRole(context.Background(), &core.Reference{
						Id: uuid.NewString(),
					})
					Expect(err).To(HaveOccurred())

					id := uuid.NewString()
					err = ts.CreateRole(context.Background(), &core.Role{
						Id: id,
					})
					Expect(err).NotTo(HaveOccurred())

					errCtrl.EnableErrors()
					defer errCtrl.DisableErrors()
					Eventually(func() error {
						_, err = ts.GetRole(context.Background(), &core.Reference{
							Id: id,
						})
						return err
					}).Should(HaveOccurred())
				})
				It("should handle errors when listing roles", func() {
					errCtrl.EnableErrors()
					defer errCtrl.DisableErrors()
					Eventually(func() error {
						_, err := ts.ListRoles(context.Background())
						return err
					}).Should(HaveOccurred())
				})
				It("should handle errors when deleting a role", func() {
					Expect(ts.DeleteRole(context.Background(), &core.Reference{
						Id: uuid.NewString(),
					})).NotTo(Succeed())
				})
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
					rb := &core.RoleBinding{
						Id: "foo",
					}
					err := ts.CreateRoleBinding(context.Background(), rb)
					Expect(err).NotTo(HaveOccurred())

					rb, err = ts.GetRoleBinding(context.Background(), rb.Reference())
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
			Context("error handling", func() {
				It("should handle errors when creating a role binding", func() {
					errCtrl.EnableErrors()
					defer errCtrl.DisableErrors()
					err := ts.CreateRoleBinding(context.Background(), &core.RoleBinding{
						Id: uuid.NewString(),
					})
					Expect(err).To(HaveOccurred())
				})
				It("should handle errors when getting a role binding", func() {
					_, err := ts.GetRoleBinding(context.Background(), &core.Reference{
						Id: uuid.NewString(),
					})
					Expect(err).To(HaveOccurred())

					id := uuid.NewString()
					err = ts.CreateRoleBinding(context.Background(), &core.RoleBinding{
						Id: id,
					})
					Expect(err).NotTo(HaveOccurred())

					errCtrl.EnableErrors()
					defer errCtrl.DisableErrors()
					Eventually(func() error {
						_, err := ts.GetRoleBinding(context.Background(), &core.Reference{
							Id: id,
						})
						return err
					}).Should(HaveOccurred())
				})
				It("should handle errors when listing role bindings", func() {
					errCtrl.EnableErrors()
					defer errCtrl.DisableErrors()
					Eventually(func() error {
						_, err := ts.ListRoleBindings(context.Background())
						return err
					}).Should(HaveOccurred())
				})
				It("should handle errors when deleting a role binding", func() {
					Expect(ts.DeleteRoleBinding(context.Background(), &core.Reference{
						Id: uuid.NewString(),
					})).NotTo(Succeed())
				})
			})
		})
	}
}
