package conformance

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/future"
)

func TokenStoreTestSuite[T storage.TokenStore](
	tsF future.Future[T],
	errCtrlF future.Future[ErrorController],
) func() {
	return func() {
		var ts T
		var errCtrl ErrorController
		BeforeAll(func() {
			ts = tsF.Get()
			errCtrl = errCtrlF.Get()
		})
		It("should initially have no tokens", func() {
			tokens, err := ts.ListTokens(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(tokens).To(BeEmpty())
		})
		When("creating a token", func() {
			var ref *corev1.Reference
			It("should be retrievable", func() {
				var tk *corev1.BootstrapToken
				Eventually(func() (err error) {
					tk, err = ts.CreateToken(context.Background(), time.Hour)
					return
				}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
				ref = tk.Reference()
				Expect(tk).NotTo(BeNil())
				Expect(tk.TokenID).NotTo(BeEmpty())
				Expect(tk.Secret).NotTo(BeEmpty())
				Expect(tk.GetMetadata().GetTtl()).To(BeNumerically("~", time.Hour.Seconds(), 1))
			})
			It("should appear in the list of tokens", func() {
				tokens, err := ts.ListTokens(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(tokens).To(HaveLen(1))
				Expect(tokens[0].GetTokenID()).To(Equal(ref.Id))
			})
			It("should be retrievable by ID", func() {
				tk, err := ts.GetToken(context.Background(), ref)
				Expect(err).NotTo(HaveOccurred())
				Expect(tk).NotTo(BeNil())
				Expect(tk.Reference().Equal(ref)).To(BeTrue())
			})
		})
		It("should handle token create options", func() {
			check := func(tk *corev1.BootstrapToken) {
				Expect(tk).NotTo(BeNil())
				Expect(tk.GetLabels()).To(HaveKeyWithValue("foo", "bar"))
				Expect(tk.GetLabels()).To(HaveKeyWithValue("bar", "baz"))
			}
			tk, err := ts.CreateToken(context.Background(), time.Hour, storage.WithLabels(
				map[string]string{
					"foo": "bar",
					"bar": "baz",
				},
			))
			Expect(err).NotTo(HaveOccurred())
			check(tk)

			tk, err = ts.GetToken(context.Background(), tk.Reference())
			Expect(err).NotTo(HaveOccurred())
			check(tk)

			list, err := ts.ListTokens(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(2))

			check = func(tk *corev1.BootstrapToken) {
				Expect(tk).NotTo(BeNil())
				Expect(tk.GetCapabilities()).To(HaveLen(1))
				Expect(tk.GetCapabilities()[0].Type).To(Equal("foo"))
				Expect(tk.GetCapabilities()[0].Reference.Id).To(Equal("bar"))
			}
			tk, err = ts.CreateToken(context.Background(), time.Hour, storage.WithCapabilities(
				[]*corev1.TokenCapability{
					{
						Type: "foo",
						Reference: &corev1.Reference{
							Id: "bar",
						},
					},
				},
			))
			Expect(err).NotTo(HaveOccurred())
			check(tk)

			tk, err = ts.GetToken(context.Background(), tk.Reference())
			Expect(err).NotTo(HaveOccurred())
			check(tk)

			list, err = ts.ListTokens(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(3))
		})
		When("deleting a token", func() {
			When("the token exists", func() {
				It("should be deleted", func() {
					tk, err := ts.CreateToken(context.Background(), time.Hour)
					Expect(err).NotTo(HaveOccurred())

					before, err := ts.ListTokens(context.Background())
					Expect(err).NotTo(HaveOccurred())

					err = ts.DeleteToken(context.Background(), tk.Reference())
					Expect(err).NotTo(HaveOccurred())

					after, err := ts.ListTokens(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(after).To(HaveLen(len(before) - 1))

					_, err = ts.GetToken(context.Background(), tk.Reference())
					Expect(err).To(MatchError(storage.ErrNotFound))
				})
			})
			When("the token does not exist", func() {
				It("should return an error", func() {
					before, err := ts.ListTokens(context.Background())
					Expect(err).NotTo(HaveOccurred())

					err = ts.DeleteToken(context.Background(), &corev1.Reference{
						Id: "doesnotexist",
					})
					Expect(err).To(MatchError(storage.ErrNotFound))

					after, err := ts.ListTokens(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(after).To(HaveLen(len(before)))
				})
			})
		})
		Context("updating tokens", func() {
			var ref *corev1.Reference
			BeforeEach(func() {
				tk, err := ts.CreateToken(context.Background(), time.Hour)
				Expect(err).NotTo(HaveOccurred())
				ref = tk.Reference()
			})

			It("should be able to increment usage count", func() {
				tk, err := ts.GetToken(context.Background(), ref)
				Expect(err).NotTo(HaveOccurred())
				oldCount := tk.GetMetadata().GetUsageCount()
				tk, err = ts.UpdateToken(context.Background(), ref,
					storage.NewIncrementUsageCountMutator())
				Expect(err).NotTo(HaveOccurred())
				Expect(tk.GetMetadata().GetUsageCount()).To(Equal(oldCount + 1))
			})
			It("should be able to add capabilities", func() {
				tk, err := ts.GetToken(context.Background(), ref)
				Expect(err).NotTo(HaveOccurred())
				oldCapabilities := tk.GetCapabilities()
				tk, err = ts.UpdateToken(context.Background(), ref,
					storage.NewAddCapabilityMutator[*corev1.BootstrapToken](&corev1.TokenCapability{
						Type: "foo",
						Reference: &corev1.Reference{
							Id: "bar",
						},
					}),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(tk.GetCapabilities()).To(HaveLen(len(oldCapabilities) + 1))
				Expect(tk.GetCapabilities()[0].Type).To(Equal("foo"))
				Expect(tk.GetCapabilities()[0].Reference.Id).To(Equal("bar"))
			})
			It("should be able to update multiple properties at once", func() {
				tk, err := ts.GetToken(context.Background(), ref)
				Expect(err).NotTo(HaveOccurred())
				oldCount := tk.GetMetadata().GetUsageCount()
				oldCapabilities := tk.GetCapabilities()
				tk, err = ts.UpdateToken(context.Background(), ref,
					storage.NewCompositeMutator(
						storage.NewIncrementUsageCountMutator(),
						storage.NewAddCapabilityMutator[*corev1.BootstrapToken](&corev1.TokenCapability{
							Type: "foo",
							Reference: &corev1.Reference{
								Id: "bar",
							},
						}),
					),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(tk.GetMetadata().GetUsageCount()).To(Equal(oldCount + 1))
				Expect(tk.GetCapabilities()).To(HaveLen(len(oldCapabilities) + 1))
				Expect(tk.GetCapabilities()[0].Type).To(Equal("foo"))
				Expect(tk.GetCapabilities()[0].Reference.Id).To(Equal("bar"))
			})
			It("should handle concurrent update requests on the same resource", func() {
				tk, err := ts.CreateToken(context.Background(), time.Hour)
				Expect(err).NotTo(HaveOccurred())

				wg := sync.WaitGroup{}
				start := make(chan struct{})
				count := testutil.IfCI(3).Else(5)
				for i := 0; i < count; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						ts.UpdateToken(context.Background(), tk.Reference(),
							storage.NewIncrementUsageCountMutator())
					}()
				}
				close(start)
				wg.Wait()

				tk, err = ts.GetToken(context.Background(), tk.Reference())
				Expect(err).NotTo(HaveOccurred())
				Expect(tk.GetMetadata().GetUsageCount()).To(Equal(int64(count)))
			})
		})
		Context("error handling", func() {
			if runtime.GOOS != "linux" {
				Skip("skipping tests on non-linux OS")
			}
			It("should handle errors when creating tokens", func() {
				errCtrl.EnableErrors()
				defer errCtrl.DisableErrors()
				Eventually(func() error {
					_, err := ts.CreateToken(context.Background(), time.Hour)
					return err
				}).Should(HaveOccurred())
			})

			It("should handle errors when getting tokens", func() {
				tk, err := ts.CreateToken(context.Background(), time.Hour)
				Expect(err).NotTo(HaveOccurred())

				errCtrl.EnableErrors()
				defer errCtrl.DisableErrors()
				Eventually(func() error {
					_, err = ts.GetToken(context.Background(), tk.Reference())
					return err
				}).Should(HaveOccurred())
			})

			It("should handle errors when listing tokens", func() {
				errCtrl.EnableErrors()
				defer errCtrl.DisableErrors()
				Eventually(func() error {
					_, err := ts.ListTokens(context.Background())
					return err
				}).Should(HaveOccurred())
			})

			It("should handle errors when updating tokens", func() {
				tk, err := ts.CreateToken(context.Background(), time.Hour)
				Expect(err).NotTo(HaveOccurred())

				errCtrl.EnableErrors()
				defer errCtrl.DisableErrors()
				Eventually(func() error {
					_, err = ts.UpdateToken(context.Background(), tk.Reference(),
						storage.NewIncrementUsageCountMutator())
					return err
				}).Should(HaveOccurred())
			})

			It("should handle errors when deleting tokens", func() {
				err := ts.DeleteToken(context.Background(), &corev1.Reference{
					Id: uuid.NewString(),
				})
				Expect(err).To(HaveOccurred())
			})
		})
	}
}
