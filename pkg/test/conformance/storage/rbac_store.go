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

func RBACStoreTestSuite[T storage.RoleBindingStore](
	tsF future.Future[T],
) func() {
	return func() {
		var ts T
		BeforeAll(func() {
			ts = tsF.Get()
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
					r.RoleIds = []string{"test-role"}
					r.Subject = "test-subject"
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(rbInfo.GetResourceVersion()).NotTo(BeEmpty())
				Expect(rbInfo.GetResourceVersion()).NotTo(Equal(prevVersion))
				if integer, err := strconv.Atoi(rbInfo.GetResourceVersion()); err == nil {
					Expect(integer).To(BeNumerically(">", util.Must(strconv.Atoi(prevVersion))))
				}
				Expect(rbInfo.RoleIds).To(Equal([]string{"test-role"}))
				Expect(rbInfo.Subject).To(Equal("test-subject"))
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
							r.RoleIds = []string{"concurrent-test-role"}
							r.Subject = "concurrent-test-subject"
						})
					}()
				}
				close(start)
				wg.Wait()

				rbInfo, err := ts.GetRoleBinding(context.Background(), rb.Reference())
				Expect(err).NotTo(HaveOccurred())
				Expect(rbInfo.RoleIds).To(Equal([]string{"concurrent-test-role"}))
				Expect(rbInfo.Subject).To(Equal("concurrent-test-subject"))
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
			It("should fail to update if the role binding does not exist", func() {
				rb := &corev1.RoleBinding{
					Id: "does-not-exist",
				}
				_, err := ts.UpdateRoleBinding(context.Background(), rb.Reference(), func(r *corev1.RoleBinding) {
					r.RoleIds = []string{"test-role"}
				})
				Expect(err).To(MatchError(storage.ErrNotFound))
			})
		})
	}
}
