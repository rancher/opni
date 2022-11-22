package conformance_storage

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/testutil"
)

func BuildTokenStoreTestSuite[T storage.TokenStore](pt *T) bool {
	return Describe("Token Store", Ordered, Label("integration", "slow"), tokenStoreTestSuite(pt))
}

func tokenStoreTestSuite[T storage.TokenStore](pt *T) func() {
	return func() {
		var t T
		BeforeAll(func() {
			t = *pt
		})
		It("should initially have no tokens", func() {
			tokens, err := t.ListTokens(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(tokens).To(BeEmpty())
		})
		When("creating a token", func() {
			var ref *corev1.Reference
			It("should be retrievable", func() {
				var tk *corev1.BootstrapToken
				Eventually(func() (err error) {
					tk, err = t.CreateToken(context.Background(), time.Hour)
					return
				}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
				ref = tk.Reference()
				Expect(tk).NotTo(BeNil())
				Expect(tk.TokenID).NotTo(BeEmpty())
				Expect(tk.Secret).NotTo(BeEmpty())
				Expect(tk.GetMetadata().GetTtl()).To(BeNumerically("~", time.Hour.Seconds(), 1))
			})
			It("should appear in the list of tokens", func() {
				tokens, err := t.ListTokens(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(tokens).To(HaveLen(1))
				Expect(tokens[0].GetTokenID()).To(Equal(ref.Id))
			})
			It("should be retrievable by ID", func() {
				tk, err := t.GetToken(context.Background(), ref)
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
			tk, err := t.CreateToken(context.Background(), time.Hour, storage.WithLabels(
				map[string]string{
					"foo": "bar",
					"bar": "baz",
				},
			))
			Expect(err).NotTo(HaveOccurred())
			check(tk)

			tk, err = t.GetToken(context.Background(), tk.Reference())
			Expect(err).NotTo(HaveOccurred())
			check(tk)

			list, err := t.ListTokens(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(2))

			check = func(tk *corev1.BootstrapToken) {
				Expect(tk).NotTo(BeNil())
				Expect(tk.GetCapabilities()).To(HaveLen(1))
				Expect(tk.GetCapabilities()[0].Type).To(Equal("foo"))
				Expect(tk.GetCapabilities()[0].Reference.Id).To(Equal("bar"))
			}
			tk, err = t.CreateToken(context.Background(), time.Hour, storage.WithCapabilities(
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

			tk, err = t.GetToken(context.Background(), tk.Reference())
			Expect(err).NotTo(HaveOccurred())
			check(tk)

			list, err = t.ListTokens(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(3))
		})
		When("deleting a token", func() {
			When("the token exists", func() {
				It("should be deleted", func() {
					tk, err := t.CreateToken(context.Background(), time.Hour)
					Expect(err).NotTo(HaveOccurred())

					before, err := t.ListTokens(context.Background())
					Expect(err).NotTo(HaveOccurred())

					err = t.DeleteToken(context.Background(), tk.Reference())
					Expect(err).NotTo(HaveOccurred())

					after, err := t.ListTokens(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(after).To(HaveLen(len(before) - 1))

					_, err = t.GetToken(context.Background(), tk.Reference())
					Expect(err).To(MatchError(storage.ErrNotFound))
				})
			})
			When("the token does not exist", func() {
				It("should return an error", func() {
					before, err := t.ListTokens(context.Background())
					Expect(err).NotTo(HaveOccurred())

					err = t.DeleteToken(context.Background(), &corev1.Reference{
						Id: "doesnotexist",
					})
					Expect(err).To(MatchError(storage.ErrNotFound))

					after, err := t.ListTokens(context.Background())
					Expect(err).NotTo(HaveOccurred())
					Expect(after).To(HaveLen(len(before)))
				})
			})
		})
		Context("updating tokens", func() {
			var ref *corev1.Reference
			BeforeEach(func() {
				tk, err := t.CreateToken(context.Background(), time.Hour)
				Expect(err).NotTo(HaveOccurred())
				ref = tk.Reference()
			})

			It("should be able to increment usage count", func() {
				tk, err := t.GetToken(context.Background(), ref)
				Expect(err).NotTo(HaveOccurred())
				oldCount := tk.GetMetadata().GetUsageCount()
				tk, err = t.UpdateToken(context.Background(), ref,
					storage.NewIncrementUsageCountMutator())
				Expect(err).NotTo(HaveOccurred())
				Expect(tk.GetMetadata().GetUsageCount()).To(Equal(oldCount + 1))
			})
			It("should be able to add capabilities", func() {
				tk, err := t.GetToken(context.Background(), ref)
				Expect(err).NotTo(HaveOccurred())
				oldCapabilities := tk.GetCapabilities()
				tk, err = t.UpdateToken(context.Background(), ref,
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
				tk, err := t.GetToken(context.Background(), ref)
				Expect(err).NotTo(HaveOccurred())
				oldCount := tk.GetMetadata().GetUsageCount()
				oldCapabilities := tk.GetCapabilities()
				tk, err = t.UpdateToken(context.Background(), ref,
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
				tk, err := t.CreateToken(context.Background(), time.Hour)
				Expect(err).NotTo(HaveOccurred())

				wg := sync.WaitGroup{}
				start := make(chan struct{})
				count := testutil.IfCI(3).Else(5)
				for i := 0; i < count; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						t.UpdateToken(context.Background(), tk.Reference(),
							storage.NewIncrementUsageCountMutator())
					}()
				}
				close(start)
				wg.Wait()

				tk, err = t.GetToken(context.Background(), tk.Reference())
				Expect(err).NotTo(HaveOccurred())
				Expect(tk.GetMetadata().GetUsageCount()).To(Equal(int64(count)))
			})
		})
	}
}
