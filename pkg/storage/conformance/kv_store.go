package conformance

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util"
)

func KeyValueStoreTestSuite[T storage.KeyValueStoreBroker](
	tsF *util.Future[T],
	errCtrlF *util.Future[ErrorController],
) func() {
	return func() {
		var ts storage.KeyValueStore
		var errCtrl ErrorController
		BeforeAll(func() {
			var err error
			ts, err = tsF.Get().KeyValueStore("test")
			Expect(err).NotTo(HaveOccurred())
			errCtrl = errCtrlF.Get()
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
		Context("error handling", func() {
			It("should return an error when deleting a non-existent key", func() {
				err := ts.Delete(context.Background(), "foo")
				Expect(err).To(HaveOccurred())
			})
			It("should return an error when getting a non-existent key", func() {
				_, err := ts.Get(context.Background(), "foo")
				Expect(err).To(HaveOccurred())
			})
			It("should return an error when specifying an invalid key", func() {
				err := ts.Delete(context.Background(), "")
				Expect(err).To(HaveOccurred())
				_, err = ts.Get(context.Background(), "")
				Expect(err).To(HaveOccurred())
				err = ts.Put(context.Background(), "", []byte(""))
				Expect(err).To(HaveOccurred())
			})
			It("should handle errors", func() {
				ts.Put(context.Background(), "foo", []byte("bar"))

				errCtrl.EnableErrors()
				defer errCtrl.DisableErrors()

				Eventually(func() error {
					_, err := ts.Get(context.Background(), "foo")
					return err
				}).Should(HaveOccurred())

				err := ts.Put(context.Background(), "bar", []byte("baz"))
				Expect(err).To(HaveOccurred())

				err = ts.Delete(context.Background(), "foo")
				Expect(err).To(HaveOccurred())

				_, err = ts.ListKeys(context.Background(), "")
				Expect(err).To(HaveOccurred())
			})
		})
	}
}
