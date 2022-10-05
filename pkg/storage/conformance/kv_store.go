package conformance

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/future"
)

func KeyValueStoreTestSuite[T storage.KeyValueStoreBroker](
	tsF future.Future[T],
) func() {
	return func() {
		var ts storage.KeyValueStore
		BeforeAll(func() {
			ts = tsF.Get().KeyValueStore("test")
		})
		It("should initially be empty", func() {
			keys, err := ts.ListKeys(context.Background(), "")
			Expect(err).NotTo(HaveOccurred())
			Expect(keys).To(BeEmpty())
		})
		When("creating a key", func() {
			It("should be retrievable", func() {
				Eventually(func() error {
					return ts.Put(context.Background(), "foo", []byte("bar"))
				}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

				value, err := ts.Get(context.Background(), "foo")
				Expect(err).NotTo(HaveOccurred())
				Expect(value).To(Equal([]byte("bar")))
			})
			It("should appear in the list of keys", func() {
				keys, err := ts.ListKeys(context.Background(), "")
				Expect(err).NotTo(HaveOccurred())
				Expect(keys).To(HaveLen(1))
				Expect(keys[0]).To(Equal("foo"))
			})
		})
		It("should delete keys", func() {
			all, err := ts.ListKeys(context.Background(), "")
			Expect(err).NotTo(HaveOccurred())
			for _, key := range all {
				err := ts.Delete(context.Background(), key)
				Expect(err).NotTo(HaveOccurred())
			}
			keys, err := ts.ListKeys(context.Background(), "")
			Expect(err).NotTo(HaveOccurred())
			Expect(keys).To(BeEmpty())
		})
	}
}
