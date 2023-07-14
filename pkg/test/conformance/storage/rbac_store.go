package conformance_storage

import (
	"context"
	"strconv"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/testruntime"
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

				role, err := ts.GetRole(context.Background(), role.Reference())
				Expect(err).NotTo(HaveOccurred())

				prevVersion := role.GetResourceVersion()
				roleInfo, err := ts.UpdateRole(context.Background(), role.Reference(), func(r *corev1.Role) {
					r.ClusterIDs = []string{"test-cluster"}
					r.MatchLabels = &corev1.LabelSelector{
						MatchLabels: map[string]string{"test-label": "test-value"},
					}
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(roleInfo.GetResourceVersion()).NotTo(BeEmpty())
				Expect(roleInfo.GetResourceVersion()).NotTo(Equal(prevVersion))
				if integer, err := strconv.Atoi(roleInfo.GetResourceVersion()); err == nil {
					Expect(integer).To(BeNumerically(">", util.Must(strconv.Atoi(prevVersion))))
				}
				Expect(roleInfo.ClusterIDs).To(Equal([]string{"test-cluster"}))
				Expect(roleInfo.GetMatchLabels().GetMatchLabels()).To(Equal(map[string]string{"test-label": "test-value"}))
			})
			It("should handle multiple concurrent update requests on the same role", func() {
				role := &corev1.Role{
					Id: "foo",
				}
				role, err := ts.GetRole(context.Background(), role.Reference())
				Expect(err).NotTo(HaveOccurred())

				wg := sync.WaitGroup{}
				start := make(chan struct{})
				count := testruntime.IfCI(5).Else(10)
				for i := 0; i < count; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						ts.UpdateRole(context.Background(), role.Reference(), func(r *corev1.Role) {
							r.ClusterIDs = []string{"updated-test-cluster"}
							r.MatchLabels = &corev1.LabelSelector{
								MatchLabels: map[string]string{"concurrent-test-label": "test-value"},
							}
						})
					}()
				}
				close(start)
				wg.Wait()

				roleInfo, err := ts.GetRole(context.Background(), role.Reference())
				Expect(err).NotTo(HaveOccurred())
				Expect(roleInfo.GetMatchLabels().GetMatchLabels()).To(Equal(map[string]string{"concurrent-test-label": "test-value"}))
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
			It("should be able to update a role binding", func() {
				rb := &corev1.RoleBinding{
					Id: "foo",
				}

				fetched, err := ts.GetRoleBinding(context.Background(), rb.Reference())
				Expect(err).NotTo(HaveOccurred())

				prevVersion := fetched.GetResourceVersion()
				rbInfo, err := ts.UpdateRoleBinding(context.Background(), rb.Reference(), func(r *corev1.RoleBinding) {
					r.RoleId = "test-role"
					r.Subjects = []string{"test-subject"}
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(rbInfo.GetResourceVersion()).NotTo(BeEmpty())
				Expect(rbInfo.GetResourceVersion()).NotTo(Equal(prevVersion))
				if integer, err := strconv.Atoi(rbInfo.GetResourceVersion()); err == nil {
					Expect(integer).To(BeNumerically(">", util.Must(strconv.Atoi(prevVersion))))
				}
				Expect(rbInfo.RoleId).To(Equal("test-role"))
				Expect(rbInfo.Subjects).To(Equal([]string{"test-subject"}))
			})
			It("should handle multiple concurrent update requests on the same role binding", func() {
				rb := &corev1.RoleBinding{
					Id: "foo",
				}
				rb, err := ts.GetRoleBinding(context.Background(), rb.Reference())
				Expect(err).NotTo(HaveOccurred())

				wg := sync.WaitGroup{}
				start := make(chan struct{})
				count := testruntime.IfCI(5).Else(10)
				for i := 0; i < count; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						ts.UpdateRoleBinding(context.Background(), rb.Reference(), func(r *corev1.RoleBinding) {
							r.RoleId = "concurrent-test-role"
							r.Subjects = []string{"concurrent-test-subject"}
						})
					}()
				}
				close(start)
				wg.Wait()

				rbInfo, err := ts.GetRoleBinding(context.Background(), rb.Reference())
				Expect(err).NotTo(HaveOccurred())
				Expect(rbInfo.RoleId).To(Equal("concurrent-test-role"))
				Expect(rbInfo.Subjects).To(Equal([]string{"concurrent-test-subject"}))
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
			It("should fail to update if the role does not exist", func() {
				rb := &corev1.RoleBinding{
					Id: "does-not-exist",
				}
				_, err := ts.UpdateRoleBinding(context.Background(), rb.Reference(), func(r *corev1.RoleBinding) {
					r.RoleId = "test-role"
				})
				Expect(err).To(MatchError(storage.ErrNotFound))
			})
		})
	}
}
