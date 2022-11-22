package conformance_storage

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/storage"
)

func BuildKeyValueStoreTestSuite[T storage.KeyValueStoreBroker](pt *T) bool {
	return Describe("Key-Value Store", Ordered, Label("integration", "slow"), keyValueStoreTestSuite(pt))
}

func keyValueStoreTestSuite[T storage.KeyValueStoreBroker](pt *T) func() {
	return func() {
		var t storage.KeyValueStore
		BeforeAll(func() {
			t = (*pt).KeyValueStore("test")
		})
		It("should initially be empty", func() {
			keys, err := t.ListKeys(context.Background(), "")
			Expect(err).NotTo(HaveOccurred())
			Expect(keys).To(BeEmpty())
		})
		When("creating a key", func() {
			It("should be retrievable", func() {
				Eventually(func() error {
					return t.Put(context.Background(), "foo", []byte("bar"))
				}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

				value, err := t.Get(context.Background(), "foo")
				Expect(err).NotTo(HaveOccurred())
				Expect(value).To(Equal([]byte("bar")))
			})
			It("should appear in the list of keys", func() {
				keys, err := t.ListKeys(context.Background(), "")
				Expect(err).NotTo(HaveOccurred())
				Expect(keys).To(HaveLen(1))
				Expect(keys[0]).To(Equal("foo"))
			})
		})
		It("should delete keys", func() {
			all, err := t.ListKeys(context.Background(), "")
			Expect(err).NotTo(HaveOccurred())
			for _, key := range all {
				err := t.Delete(context.Background(), key)
				Expect(err).NotTo(HaveOccurred())
			}
			keys, err := t.ListKeys(context.Background(), "")
			Expect(err).NotTo(HaveOccurred())
			Expect(keys).To(BeEmpty())
		})
	}
}
