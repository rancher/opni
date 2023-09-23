package inmemory_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
)

var _ = Describe("InMemoryKeyValueStore", Label("unit"), func() {
	var (
		keyValueStore storage.KeyValueStoreT[string]
		ctx           context.Context
	)

	BeforeEach(func() {
		cloneFunc := func(val string) string {
			return val
		}
		keyValueStore = inmemory.NewKeyValueStore(cloneFunc)
		ctx = context.Background()
	})

	Describe("Put operation", func() {
		When("putting a value with a valid key", func() {
			It("should not return an error", func() {
				err := keyValueStore.Put(ctx, "key1", "value1")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("putting a value with an empty key", func() {
			It("should return an error", func() {
				err := keyValueStore.Put(ctx, "", "value1")
				Expect(err).To(Equal(status.Errorf(codes.InvalidArgument, "key cannot be empty")))
			})
		})
	})

	Describe("Get operation", func() {
		When("getting a value for a valid existing key", func() {
			It("should return the value without an error", func() {
				Expect(keyValueStore.Put(ctx, "key1", "value1")).To(Succeed())
				value, err := keyValueStore.Get(ctx, "key1")
				Expect(err).NotTo(HaveOccurred())
				Expect(value).To(Equal("value1"))
			})
		})

		When("getting a value for a non-existent key", func() {
			It("should return an error", func() {
				_, err := keyValueStore.Get(ctx, "nonexistent")
				Expect(err).To(Equal(storage.ErrNotFound))
			})
		})
	})

	Describe("Delete operation", func() {
		When("deleting a value for an existing key", func() {
			It("should not return an error", func() {
				Expect(keyValueStore.Put(ctx, "key1", "value1")).To(Succeed())
				err := keyValueStore.Delete(ctx, "key1")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("deleting a value for a non-existent key", func() {
			It("should return an error", func() {
				err := keyValueStore.Delete(ctx, "nonexistent")
				Expect(err).To(Equal(storage.ErrNotFound))
			})
		})
	})

	Describe("ListKeys operation", func() {
		When("listing keys with a specific prefix", func() {
			It("should return the correct keys", func() {
				Expect(keyValueStore.Put(ctx, "key1", "value1")).To(Succeed())
				Expect(keyValueStore.Put(ctx, "key2", "value2")).To(Succeed())
				Expect(keyValueStore.Put(ctx, "abc", "value3")).To(Succeed())
				keys, err := keyValueStore.ListKeys(ctx, "key")
				Expect(err).NotTo(HaveOccurred())
				Expect(keys).To(ConsistOf("key1", "key2"))
			})
		})

		When("some keys have been deleted", func() {
			It("should not include the deleted keys", func() {
				Expect(keyValueStore.Put(ctx, "key1", "value1")).To(Succeed())
				Expect(keyValueStore.Put(ctx, "key2", "value2")).To(Succeed())
				Expect(keyValueStore.Delete(ctx, "key1")).To(Succeed())

				Expect(keyValueStore.Put(ctx, "key3", "value3")).To(Succeed())
				Expect(keyValueStore.Delete(ctx, "key3")).To(Succeed())

				keys, err := keyValueStore.ListKeys(ctx, "key")
				Expect(err).NotTo(HaveOccurred())
				Expect(keys).To(ConsistOf("key2"))
			})
		})
	})

	Describe("History operation", func() {
		When("getting history for an existing key", func() {
			It("should return the history up to the specified revision", func() {
				Expect(keyValueStore.Put(ctx, "key1", "value1")).To(Succeed())
				Expect(keyValueStore.Put(ctx, "key1", "value2")).To(Succeed())
				Expect(keyValueStore.Delete(ctx, "key1")).To(Succeed())
				_, err := keyValueStore.History(ctx, "key1")
				Expect(err).To(Equal(storage.ErrNotFound))

				history, err := keyValueStore.History(ctx, "key1", storage.WithRevision(2))
				Expect(err).NotTo(HaveOccurred())
				Expect(len(history)).To(Equal(2))

				history, err = keyValueStore.History(ctx, "key1", storage.WithRevision(1))
				Expect(err).NotTo(HaveOccurred())
				Expect(len(history)).To(Equal(1))

				_, err = keyValueStore.History(ctx, "key1", storage.WithRevision(0))
				Expect(err).To(Equal(storage.ErrNotFound))

				_, err = keyValueStore.History(ctx, "key1", storage.WithRevision(-1))
				Expect(err).To(Equal(storage.ErrNotFound))
			})
		})

		When("getting history for a non-existent key", func() {
			It("should return an error", func() {
				_, err := keyValueStore.History(ctx, "nonexistent")
				Expect(err).To(Equal(storage.ErrNotFound))
			})
		})
	})
})
