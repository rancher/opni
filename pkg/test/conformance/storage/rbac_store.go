package conformance_storage

import (
	"context"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
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
			It("should be able to update a role", func() {
				role := &corev1.Role{
					Id: "foo",
				}

				fetched, err := ts.GetRole(context.Background(), role.Reference())
				Expect(err).NotTo(HaveOccurred())

				prevVersion := fetched.GetResourceVersion()
				actual, err := ts.UpdateRole(context.Background(), role.Reference(), func(r *corev1.Role) {
					r.ClusterIDs = []string{"test-cluster"}
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual.GetResourceVersion()).NotTo(BeEmpty())
				Expect(actual.GetResourceVersion()).NotTo(Equal(prevVersion))
				if integer, err := strconv.Atoi(actual.GetResourceVersion()); err == nil {
					Expect(integer).To(BeNumerically(">", util.Must(strconv.Atoi(prevVersion))))
				}

				role, err = ts.GetRole(context.Background(), role.Reference())
				Expect(err).NotTo(HaveOccurred())
				Expect(role.ClusterIDs).To(Equal([]string{"test-cluster"}))
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
			It("should fail if the role already exists", func() {
				role := &corev1.Role{
					Id: "foo",
				}
				err := ts.CreateRole(context.Background(), role)
				Expect(err).NotTo(HaveOccurred())

				err = ts.CreateRole(context.Background(), role)
				Expect(err).To(MatchError(storage.ErrAlreadyExists))
			})
			It("should fail to update if the role does not exist", func() {
				role := &corev1.Role{
					Id: "does-not-exist",
				}
				_, err := ts.UpdateRole(context.Background(), role.Reference(), func(r *corev1.Role) {
					r.ClusterIDs = []string{"test-cluster"}
				})
				Expect(err).To(MatchError(storage.ErrNotFound))
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
			It("should fail if the role binding already exists", func() {
				rb := &corev1.RoleBinding{
					Id: "foo",
				}
				err := ts.CreateRoleBinding(context.Background(), rb)
				Expect(err).NotTo(HaveOccurred())

				err = ts.CreateRoleBinding(context.Background(), rb)
				Expect(err).To(MatchError(storage.ErrAlreadyExists))
			})
		})
	}
}
