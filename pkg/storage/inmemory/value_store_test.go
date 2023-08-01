package inmemory_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Value Store", func() {
	var (
		ctx        context.Context
		valueStore storage.ValueStoreT[string]
		updateC    chan lo.Tuple2[string, string]
	)

	BeforeEach(func() {
		updateC = make(chan lo.Tuple2[string, string], 10)

		valueStore = inmemory.NewValueStore(strings.Clone, func(prev, value string) {
			updateC <- lo.T2(prev, value)
		})
		ctx = context.TODO()
	})

	Describe("Put", func() {
		When("a value is put without a specific revision", func() {
			It("should put the value successfully", func() {
				Expect(valueStore.Put(ctx, "value")).To(Succeed())
				value, err := valueStore.Get(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(value).To(Equal("value"))
			})
		})

		When("a value is put with a specific revision", func() {
			It("should fail if the revision does not match", func() {
				rev := int64(10)
				Expect(valueStore.Put(ctx, "value", storage.WithRevision(rev))).To(HaveOccurred())
			})

			It("should succeed if the revision matches", func() {
				revOut := int64(0)
				Expect(valueStore.Put(ctx, "value")).To(Succeed())
				Expect(valueStore.Put(ctx, "new-value", storage.WithRevisionOut(&revOut))).To(Succeed())
				Expect(valueStore.Put(ctx, "new-value2", storage.WithRevision(revOut))).To(Succeed())
				value, err := valueStore.Get(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(value).To(Equal("new-value2"))
			})
		})
	})

	Describe("Get", func() {
		When("retrieving a value that does not exist", func() {
			It("should return an error", func() {
				_, err := valueStore.Get(ctx)
				Expect(err).To(Equal(storage.ErrNotFound))
			})
		})

		When("retrieving a value with a specific revision", func() {
			It("should retrieve the value for the correct revision", func() {
				revOut := int64(0)
				Expect(valueStore.Put(ctx, "value1")).To(Succeed())
				Expect(valueStore.Put(ctx, "value2", storage.WithRevisionOut(&revOut))).To(Succeed())
				value, err := valueStore.Get(ctx, storage.WithRevision(revOut))
				Expect(err).NotTo(HaveOccurred())
				Expect(value).To(Equal("value2"))
			})
			When("the value at the specified revision has been deleted", func() {
				It("should return a NotFound error", func() {
					Expect(valueStore.Put(ctx, "value1")).To(Succeed())
					Expect(valueStore.Put(ctx, "value2")).To(Succeed())
					Expect(valueStore.Delete(ctx)).To(Succeed())
					Expect(valueStore.Put(ctx, "value3")).To(Succeed())
					_, err := valueStore.Get(ctx, storage.WithRevision(3))
					Expect(err).To(Equal(storage.ErrNotFound))
				})
			})
		})
	})

	Describe("Delete", func() {
		When("deleting a value that does not exist", func() {
			It("should return an error", func() {
				Expect(valueStore.Delete(ctx)).To(Equal(storage.ErrNotFound))
			})
		})

		When("deleting a value with a specific revision", func() {
			It("should delete the value if the revision matches", func() {
				Expect(valueStore.Put(ctx, "value")).To(Succeed())
				Expect(valueStore.Delete(ctx, storage.WithRevision(1))).To(Succeed())
				_, err := valueStore.Get(ctx)
				Expect(err).To(Equal(storage.ErrNotFound))
			})

			It("should not delete the value if the revision does not match", func() {
				rev := int64(10)
				Expect(valueStore.Put(ctx, "value")).To(Succeed())
				Expect(valueStore.Delete(ctx, storage.WithRevision(rev))).To(HaveOccurred())
				value, err := valueStore.Get(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(value).To(Equal("value"))
			})
		})

		When("deleting a value that has been marked as deleted", func() {
			It("should return an error", func() {
				Expect(valueStore.Put(ctx, "value")).To(Succeed())
				Expect(valueStore.Delete(ctx)).To(Succeed())
				Expect(valueStore.Delete(ctx)).To(Equal(storage.ErrNotFound))
			})
		})

		When("deleting a value that doesn't exist", func() {
			It("should return an error", func() {
				Expect(valueStore.Delete(ctx)).To(Equal(storage.ErrNotFound))
			})
		})
	})

	Describe("History", func() {
		When("retrieving history for a non-existent key", func() {
			It("should return an error", func() {
				_, err := valueStore.History(ctx)
				Expect(err).To(Equal(storage.ErrNotFound))
			})
		})

		When("retrieving history with a specific revision", func() {
			It("should retrieve the history up to the specified revision", func() {
				revOut := int64(0)
				Expect(valueStore.Put(ctx, "value1")).To(Succeed())
				Expect(valueStore.Put(ctx, "value2", storage.WithRevisionOut(&revOut))).To(Succeed())
				Expect(valueStore.Put(ctx, "value3")).To(Succeed())
				history, err := valueStore.History(ctx, storage.WithRevision(revOut))
				Expect(err).NotTo(HaveOccurred())
				Expect(len(history)).To(Equal(2))
			})
		})

		When("retrieving history with a revision that does not exist", func() {
			It("should return a NotFound error", func() {
				_, err := valueStore.History(ctx, storage.WithRevision(-1))
				Expect(err).To(Equal(storage.ErrNotFound))
			})
		})

		When("retrieving history where the current element has been deleted", func() {
			It("should return a NotFound error", func() {
				Expect(valueStore.Put(ctx, "value1")).To(Succeed())
				Expect(valueStore.Put(ctx, "value2")).To(Succeed())
				Expect(valueStore.Delete(ctx)).To(Succeed())
				_, err := valueStore.History(ctx, storage.IncludeValues(true), storage.WithRevision(3))
				Expect(err).To(Equal(storage.ErrNotFound))
			})

			When("a new element is added after the deleted element", func() {
				It("should not include the deleted element in the history", func() {
					Expect(valueStore.Put(ctx, "value1")).To(Succeed())
					Expect(valueStore.Put(ctx, "value2")).To(Succeed())
					Expect(valueStore.Delete(ctx)).To(Succeed())

					Expect(valueStore.Put(ctx, "value3")).To(Succeed())
					history, err := valueStore.History(ctx, storage.IncludeValues(true))
					Expect(err).NotTo(HaveOccurred())
					Expect(len(history)).To(Equal(1))
					Expect(history[0].Value()).To(Equal("value3"))
				})
			})
		})
	})

	Specify("NewProtoValueStore should construct a new store with listeners", func() {
		// todo: this type is a workaround for a bug in the current go 1.21 rc
		l := func(prev, value *emptypb.Empty) {}
		_ = inmemory.NewProtoValueStore[*emptypb.Empty](l)
		_ = inmemory.NewProtoValueStore[*emptypb.Empty](l, l)
		_ = inmemory.NewProtoValueStore[*emptypb.Empty](l, l, l)

		// the following line does not compile
		// inmemory.NewValueStore(proto.Clone, l)
	})
})
