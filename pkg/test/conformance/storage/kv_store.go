package conformance_storage

import (
	"context"
	"encoding/json"
	"fmt"
	mathrand "math/rand"
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
)

func NewBytes(seed ...int64) []byte {
	if len(seed) == 0 {
		return nil
	}
	src := mathrand.NewSource(seed[0])
	bytes := make([]byte, 64)
	for i := range bytes {
		bytes[i] = byte(src.Int63() % 256)
	}
	return bytes
}

// The function [newT] must return a new T according to the following rules:
// 1. newT(a) == newT(a)
// 2. newT(a) != newT(b)
// 3. newT() == zero
func KeyValueStoreTestSuite[B storage.KeyValueStoreTBroker[T], T any](
	tsF future.Future[B],
	newT func(seed ...int64) T,
	equal func(any) types.GomegaMatcher,
) func() {
	// sanity-check the newT function
	values := map[string]int64{}
	for i := 0; i < 100; i++ {
		t := newT(int64(i))
		encoded, err := json.Marshal(t)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal newT(%d): %v", i, err))
		}
		if _, ok := values[string(encoded)]; ok {
			panic(fmt.Sprintf("newT(i) == newT(j) for i=%d, j=%d", i, values[string(encoded)]))
		}
		values[string(encoded)] = int64(i)
	}
	if !reflect.DeepEqual(newT(), lo.Empty[T]()) {
		panic("newT() != nil")
	}

	return func() {
		Context("basic operations", func() {
			var ts storage.KeyValueStoreT[T]
			BeforeAll(func() {
				ts = tsF.Get().KeyValueStore("basic")
			})
			It("should initially be empty", func() {
				keys, err := ts.ListKeys(context.Background(), "")
				Expect(err).NotTo(HaveOccurred())
				Expect(keys).To(BeEmpty())

				value, err := ts.Get(context.Background(), "foo")
				Expect(err).To(testutil.MatchStatusCode(codes.NotFound))
				Expect(value).To(BeNil())

				err = ts.Delete(context.Background(), "foo")
				Expect(err).To(testutil.MatchStatusCode(codes.NotFound))
			})
			When("creating a key", func() {
				It("should be retrievable", func() {
					Eventually(func() error {
						return ts.Put(context.Background(), "foo", newT(1))
					}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

					value, err := ts.Get(context.Background(), "foo")
					Expect(err).NotTo(HaveOccurred())
					Expect(value).To(equal(newT(1)))
				})
				It("should appear in the list of keys", func() {
					keys, err := ts.ListKeys(context.Background(), "")
					Expect(err).NotTo(HaveOccurred())
					Expect(keys).To(HaveLen(1))
					Expect(keys[0]).To(Equal("foo"))
				})
				When("an invalid key is used", func() {
					It("should return an InvalidArgument error", func() {
						err := ts.Put(context.Background(), "", newT(2))
						Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument))
					})
				})
				When("putting a nil value", func() {
					It("should treat it as the zero value for that type", func() {
						err := ts.Put(context.Background(), "foo", newT())
						Expect(err).NotTo(HaveOccurred())

						value, err := ts.Get(context.Background(), "foo")
						Expect(err).NotTo(HaveOccurred())
						var zero T
						if val := reflect.ValueOf(newT()); val.IsNil() {
							if val.Kind() == reflect.Ptr {
								zero = reflect.New(val.Type().Elem()).Interface().(T)
							} else {
								zero = reflect.Zero(val.Type()).Interface().(T)
							}
						}
						Expect(value).To(equal(zero))
					})
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
			When("accessing a deleted key", func() {
				It("should return a NotFound error", func() {
					ts.Put(context.Background(), "foo", newT(1))
					err := ts.Delete(context.Background(), "foo")
					Expect(err).NotTo(HaveOccurred())

					value, err := ts.Get(context.Background(), "foo")
					Expect(err).To(testutil.MatchStatusCode(codes.NotFound))
					Expect(value).To(BeNil())

					err = ts.Delete(context.Background(), "foo")
					Expect(err).To(testutil.MatchStatusCode(codes.NotFound))

					keys, err := ts.ListKeys(context.Background(), "")
					Expect(err).NotTo(HaveOccurred())
					Expect(keys).To(BeEmpty())

					hist, err := ts.History(context.Background(), "foo")
					Expect(err).To(testutil.MatchStatusCode(codes.NotFound))
					Expect(hist).To(BeEmpty())
				})
			})
			When("an invalid key is used", func() {
				It("should return an InvalidArgument error", func() {
					_, err := ts.Get(context.Background(), "")
					Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument))
					err = ts.Put(context.Background(), "", newT(3))
					Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument))
					err = ts.Delete(context.Background(), "")
					Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument))
				})
			})
		})
		Context("key revisions", func() {
			var ts storage.KeyValueStoreT[T]

			BeforeEach(func() {
				ts = tsF.Get().KeyValueStore(uuid.NewString())
				// add some random keys to increase the revision
				for i := 0; i < 5; i++ {
					for j := 0; j < 5; j++ {
						Expect(ts.Put(context.Background(), fmt.Sprintf("testkey%d", i), newT(int64(j)))).To(Succeed())
					}
				}
				for i := 0; i < 5; i++ {
					Expect(ts.Delete(context.Background(), fmt.Sprintf("testkey%d", i))).To(Succeed())
				}
			})

			When("using revision with Put", func() {
				It("should only put if the revision matches", func() {
					var revision1 int64
					Expect(ts.Put(context.Background(), "key1", newT(1), storage.WithRevisionOut(&revision1))).To(Succeed())
					var revision2 int64
					Expect(ts.Put(context.Background(), "key1", newT(2), storage.WithRevisionOut(&revision2))).To(Succeed())

					// Check with wrong revision
					Expect(ts.Put(context.Background(), "key1", newT(3), storage.WithRevision(revision1))).To(testutil.MatchStatusCode(storage.ErrConflict))

					value, err := ts.Get(context.Background(), "key1")
					Expect(err).NotTo(HaveOccurred())
					Expect(value).To(equal(newT(2)))

					// Check with correct revision
					Expect(ts.Put(context.Background(), "key1", newT(3), storage.WithRevision(revision2))).To(Succeed())

					value, err = ts.Get(context.Background(), "key1")
					Expect(err).NotTo(HaveOccurred())
					Expect(value).To(equal(newT(3)))
				})
				When("specifying future revisions", func() {
					It("should return a conflict error", func() {
						var revision int64
						Expect(ts.Put(context.Background(), "key1", newT(1), storage.WithRevisionOut(&revision))).To(Succeed())
						Expect(ts.Put(context.Background(), "key1", newT(2), storage.WithRevision(revision+1))).To(testutil.MatchStatusCode(storage.ErrConflict))
					})
				})
			})

			When("using revision with Get", func() {
				It("should retrieve the value at the specific revision", func() {
					revisions := make([]int64, 10)
					for i := 0; i < 10; i++ {
						err := ts.Put(context.Background(), "key1", newT(int64(i)), storage.WithRevisionOut(&revisions[i]))
						Expect(err).NotTo(HaveOccurred())
					}
					for i := 0; i < 10; i++ {
						value, err := ts.Get(context.Background(), "key1", storage.WithRevision(revisions[i]))
						Expect(err).NotTo(HaveOccurred())
						Expect(value).To(equal(newT(int64(i))))
					}
				})
				When("the revision does not exist", func() {
					When("the revision is a future revision", func() {
						It("should return an OutOfRange error", func() {
							var revision int64
							err := ts.Put(context.Background(), "key1", newT(1), storage.WithRevisionOut(&revision))
							Expect(err).NotTo(HaveOccurred())
							_, err = ts.Get(context.Background(), "key1", storage.WithRevision(revision+1))
							Expect(err).To(testutil.MatchStatusCode(codes.OutOfRange))
						})
					})
					When("the revision is a past revision", func() {
						It("should return a NotFound error", func() {
							var revision int64
							Expect(ts.Put(context.Background(), "key1", newT(1), storage.WithRevisionOut(&revision))).To(Succeed())
							var sameRevision int64
							v, err := ts.Get(context.Background(), "key1", storage.WithRevision(revision), storage.WithRevisionOut(&sameRevision))
							Expect(err).NotTo(HaveOccurred())
							Expect(sameRevision).To(Equal(revision))
							Expect(v).To(equal(newT(1)))
							_, err = ts.Get(context.Background(), "key1", storage.WithRevision(revision-1))
							Expect(err).To(testutil.MatchStatusCode(codes.NotFound))
						})
					})
				})
				When("the revision is not specified", func() {
					It("should return the latest value", func() {
						err := ts.Put(context.Background(), "key1", newT(2))
						Expect(err).NotTo(HaveOccurred())

						value, err := ts.Get(context.Background(), "key1")
						Expect(err).NotTo(HaveOccurred())
						Expect(value).To(equal(newT(2)))
					})
				})
			})

			When("using revision with Delete", func() {
				It("should delete if the revision matches", func() {
					var revision1 int64
					Expect(ts.Put(context.Background(), "key1", newT(1), storage.WithRevisionOut(&revision1))).To(Succeed())

					var revision2 int64
					Expect(ts.Put(context.Background(), "key1", newT(2), storage.WithRevisionOut(&revision2))).To(Succeed())

					Expect(ts.Delete(context.Background(), "key1", storage.WithRevision(revision1))).To(testutil.MatchStatusCode(storage.ErrConflict))

					value, err := ts.Get(context.Background(), "key1")
					Expect(err).NotTo(HaveOccurred())
					Expect(value).To(equal(newT(2)))

					Expect(ts.Delete(context.Background(), "key1", storage.WithRevision(revision2))).To(Succeed())

					_, err = ts.Get(context.Background(), "key1")
					Expect(err).To(testutil.MatchStatusCode(codes.NotFound))
				})
			})

			When("using limit with ListKeys", func() {
				It("should return the keys limited by the specified number", func() {
					keys, err := ts.ListKeys(context.Background(), "asdf/")
					Expect(err).NotTo(HaveOccurred())
					Expect(keys).To(HaveLen(0))

					for i := 0; i < 10; i++ {
						err := ts.Put(context.Background(), fmt.Sprintf("asdf/key%d", i), newT(int64(i)))
						Expect(err).NotTo(HaveOccurred())
					}

					keys, err = ts.ListKeys(context.Background(), "asdf/", storage.WithLimit(5))
					Expect(err).NotTo(HaveOccurred())
					Expect(keys).To(HaveLen(5))

					keys, err = ts.ListKeys(context.Background(), "asdf/", storage.WithLimit(15))
					Expect(err).NotTo(HaveOccurred())
					Expect(keys).To(HaveLen(10))

					keys, err = ts.ListKeys(context.Background(), "asdf/")
					Expect(err).NotTo(HaveOccurred())
					Expect(keys).To(HaveLen(10))
				})
			})
			Context("History", func() {
				It("should store key history", SpecTimeout(10*time.Second), func(ctx context.Context) {
					wg := sync.WaitGroup{}
					for i := 0; i < 10; i++ {
						revisions := make([]int64, 10)
						for j := 0; j < 10; j++ {
							i, j := i, j
							var revision int64
							Expect(ts.Put(ctx, fmt.Sprintf("history/key%d", i), newT(int64(j)), storage.WithRevisionOut(&revision))).To(Succeed())
							revisions[j] = revision
							wg.Add(1)
							go func() {
								defer GinkgoRecover()
								defer wg.Done()
								revs, err := ts.History(ctx, fmt.Sprintf("history/key%d", i), storage.WithRevision(revision))
								Expect(err).NotTo(HaveOccurred())
								Expect(revs).To(HaveLen(j + 1))
								for k := 0; k <= j; k++ {
									Expect(revs[k].Key()).To(Equal(fmt.Sprintf("history/key%d", i)))
									Expect(revs[k].Value()).To(BeNil())
									Expect(revs[k].Revision()).To(Equal(revisions[k]))
									if k > 0 {
										Expect(revs[k].Timestamp()).To(Or(
											BeZero(),
											BeTemporally(">", revs[k-1].Timestamp()),
										))
									}
								}
							}()
							wg.Add(1)
							go func() {
								defer GinkgoRecover()
								defer wg.Done()
								revs, err := ts.History(ctx, fmt.Sprintf("history/key%d", i), storage.IncludeValues(true), storage.WithRevision(revision))
								Expect(err).NotTo(HaveOccurred())
								Expect(revs).To(HaveLen(j + 1))
								for k := 0; k <= j; k++ {
									Expect(revs[k].Key()).To(Equal(fmt.Sprintf("history/key%d", i)))
									Expect(revs[k].Value()).To(equal(newT(int64(k))))
									Expect(revs[k].Revision()).To(Equal(revisions[k]))
									if k > 0 {
										Expect(revs[k].Timestamp()).To(Or(
											BeZero(),
											BeTemporally(">", revs[k-1].Timestamp()),
										))
									}
								}
							}()
						}
					}
					done := make(chan struct{})
					go func() {
						wg.Wait()
						close(done)
					}()
					Eventually(ctx, done).Should(BeClosed())
				})
				When("the key is not found", func() {
					It("should return a NotFound error", func() {
						_, err := ts.History(context.Background(), "asdf")
						Expect(err).To(testutil.MatchStatusCode(codes.NotFound))
					})
				})
				When("an invalid key is used", func() {
					It("should return an InvalidArgument error", func() {
						_, err := ts.History(context.Background(), "")
						Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument))
					})
				})
				When("a key has history and is deleted", func() {
					It("should allow accessing history using an older revision", func() {
						var revision int64
						Expect(ts.Put(context.Background(), "key1", newT(1))).To(Succeed())
						Expect(ts.Put(context.Background(), "key1", newT(2))).To(Succeed())
						Expect(ts.Put(context.Background(), "key1", newT(3), storage.WithRevisionOut(&revision))).To(Succeed())
						Expect(ts.Delete(context.Background(), "key1")).To(Succeed())

						revs, err := ts.History(context.Background(), "key1", storage.WithRevision(revision), storage.IncludeValues(true))
						if util.StatusCode(err) == codes.Unimplemented {
							Skip(status.Convert(err).Message())
						}
						Expect(err).NotTo(HaveOccurred())
						Expect(revs).To(HaveLen(3))
						Expect(revs[0].Value()).To(equal(newT(1)))
						Expect(revs[1].Value()).To(equal(newT(2)))
						Expect(revs[2].Value()).To(equal(newT(3)))
					})
				})
			})
		})
	}
}
