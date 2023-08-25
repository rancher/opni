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
	match func(any) types.GomegaMatcher,
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
					Expect(value).To(match(newT(1)))
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
				When("the key does not exist", func() {
					When("revision 0 is provided", func() {
						It("should only create the key if it does not exist or has been deleted", func() {
							uniqueKey := uuid.NewString()
							var revision int64
							Expect(ts.Put(context.Background(), uniqueKey, newT(1), storage.WithRevisionOut(&revision))).To(Succeed())
							err := ts.Put(context.Background(), uniqueKey, newT(2), storage.WithRevision(0))
							Expect(storage.IsConflict(err)).To(BeTrue(), err.Error())
							Expect(ts.Delete(context.Background(), uniqueKey)).To(Succeed())
							Expect(ts.Put(context.Background(), uniqueKey, newT(2), storage.WithRevision(0))).To(Succeed())
						})
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
						Expect(value).To(match(zero))
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
		Context("Watch", func() {
			var ts storage.KeyValueStoreT[T]
			BeforeEach(func() {
				ts = tsF.Get().KeyValueStore(uuid.NewString())
			})

			matchEvent := func(eventC <-chan storage.WatchEvent[storage.KeyRevision[T]], eventType storage.WatchEventType, key string, prev T, prevRev int64, current T, currentRev int64) {
				GinkgoHelper()
				var event storage.WatchEvent[storage.KeyRevision[T]]
				select {
				case <-time.After(1000 * time.Second):
					Fail("timed out waiting for event")
				case event = <-eventC:
				}
				// Eventually(eventC).Should(Receive(&event))
				Expect(event.EventType).To(Equal(eventType))

				note := fmt.Sprintf("type=%s,key=%s,prev=%v,prevRev=%d,current=%v,currentRev=%d", eventType, key, prev, prevRev, current, currentRev)
				switch event.EventType {
				case storage.WatchEventPut:
					Expect(event.Current).NotTo(BeNil(), note)
					Expect(event.Current.Key()).To(Equal(key), note)
					Expect(event.Current.Value()).To(match(current), note)
					Expect(event.Current.Revision()).To(Equal(currentRev), note)
					if !reflect.ValueOf(prev).IsNil() {
						Expect(event.Previous).NotTo(BeNil(), note)
						Expect(event.Previous.Key()).To(Equal(key), note)
						Expect(event.Previous.Value()).To(match(prev), note)
						Expect(event.Previous.Revision()).To(Equal(prevRev), note)
					}
				case storage.WatchEventDelete:
					Expect(event.Current).To(BeNil(), note)
					Expect(event.Previous).NotTo(BeNil(), note)
					Expect(event.Previous.Key()).To(Equal(key), note)
					Expect(event.Previous.Value()).To(match(prev), note)
					Expect(event.Previous.Revision()).To(Equal(prevRev), note)
				}
			}
			var none T
			var noRev int64 = -1

			It("should watch for changes to keys", func(ctx SpecContext) {
				updateC, err := ts.Watch(ctx, "key")
				Expect(err).NotTo(HaveOccurred())

				By("creating the key")
				var revisions [4]int64
				Expect(ts.Put(ctx, "key", newT(1), storage.WithRevisionOut(&revisions[0]))).To(Succeed())
				matchEvent(updateC, storage.WatchEventPut, "key", none, noRev, newT(1), revisions[0])

				By("updating the key")
				Expect(ts.Put(ctx, "key", newT(2), storage.WithRevisionOut(&revisions[1]))).To(Succeed())
				matchEvent(updateC, storage.WatchEventPut, "key", newT(1), revisions[0], newT(2), revisions[1])

				By("deleting the key")
				Expect(ts.Delete(ctx, "key")).To(Succeed())
				matchEvent(updateC, storage.WatchEventDelete, "key", newT(2), revisions[1], none, noRev)

				By("recreating the key")
				Expect(ts.Put(ctx, "key", newT(4), storage.WithRevisionOut(&revisions[2]))).To(Succeed())
				matchEvent(updateC, storage.WatchEventPut, "key", none, noRev, newT(4), revisions[2])

				By("updating the key")
				Expect(ts.Put(ctx, "key", newT(5), storage.WithRevisionOut(&revisions[3]))).To(Succeed())
				matchEvent(updateC, storage.WatchEventPut, "key", newT(4), revisions[2], newT(5), revisions[3])
			})

			It("should watch for changes to keys by prefix", func(ctx SpecContext) {
				updateC, err := ts.Watch(ctx, "prefix", storage.WithPrefix())
				Expect(err).NotTo(HaveOccurred())
				var revisions [4]int64

				Expect(ts.Put(ctx, "prefix/key1", newT(1), storage.WithRevisionOut(&revisions[0]))).To(Succeed())
				matchEvent(updateC, storage.WatchEventPut, "prefix/key1", none, noRev, newT(1), revisions[0])

				Expect(ts.Put(ctx, "prefix/key1", newT(2), storage.WithRevisionOut(&revisions[1]))).To(Succeed())
				matchEvent(updateC, storage.WatchEventPut, "prefix/key1", newT(1), revisions[0], newT(2), revisions[1])

				Expect(ts.Delete(ctx, "prefix/key1")).To(Succeed())
				matchEvent(updateC, storage.WatchEventDelete, "prefix/key1", newT(2), revisions[1], none, noRev)

				Expect(ts.Put(ctx, "prefix/key2", newT(3), storage.WithRevisionOut(&revisions[2]))).To(Succeed())
				matchEvent(updateC, storage.WatchEventPut, "prefix/key2", none, noRev, newT(3), revisions[2])

				By("putting a key with a non-matching prefix")
				Expect(ts.Put(ctx, "otherprefix/key", newT(4))).To(Succeed())
				Consistently(updateC).WithTimeout(10 * time.Millisecond).ShouldNot(Receive())
			})

			It("should watch for changes to keys with a starting revision", func(ctx SpecContext) {
				By("creating the key")
				var revisions [2]int64
				Expect(ts.Put(ctx, "prefix/key", newT(1), storage.WithRevisionOut(&revisions[0]))).To(Succeed())
				Expect(ts.Put(ctx, "prefix/key", newT(2), storage.WithRevisionOut(&revisions[1]))).To(Succeed())

				By("watching for changes starting from a previous revision")
				updateC, err := ts.Watch(ctx, "prefix/key", storage.WithRevision(revisions[0]))
				Expect(err).NotTo(HaveOccurred())
				matchEvent(updateC, storage.WatchEventPut, "prefix/key", none, noRev, newT(1), revisions[0])
				matchEvent(updateC, storage.WatchEventPut, "prefix/key", newT(1), revisions[0], newT(2), revisions[1])
			})

			It("should watch for changes to keys with a starting revision by prefix", func(ctx SpecContext) {
				By("creating the key")
				var revisions [2]int64
				Expect(ts.Put(context.Background(), "prefix/key", newT(1), storage.WithRevisionOut(&revisions[0]))).To(Succeed())
				Expect(ts.Put(context.Background(), "prefix/key", newT(2), storage.WithRevisionOut(&revisions[1]))).To(Succeed())

				By("watching for changes starting from a previous revision")
				updateC, err := ts.Watch(ctx, "prefix", storage.WithRevision(revisions[0]), storage.WithPrefix())
				Expect(err).NotTo(HaveOccurred())
				matchEvent(updateC, storage.WatchEventPut, "prefix/key", none, noRev, newT(1), revisions[0])
				matchEvent(updateC, storage.WatchEventPut, "prefix/key", newT(1), revisions[0], newT(2), revisions[1])
			})

			It("should duplicate events to multiple watchers with mixed options", func(ctx SpecContext) {
				By("creating the key")
				var revisions [3]int64
				Expect(ts.Put(context.Background(), "key", newT(1), storage.WithRevisionOut(&revisions[0]))).To(Succeed())
				Expect(ts.Put(context.Background(), "key", newT(2), storage.WithRevisionOut(&revisions[1]))).To(Succeed())

				By("watching for changes starting from a previous revision")
				updateC1, err := ts.Watch(ctx, "key", storage.WithRevision(revisions[0]))
				Expect(err).NotTo(HaveOccurred())
				updateC2, err := ts.Watch(ctx, "key", storage.WithRevision(revisions[1]))
				Expect(err).NotTo(HaveOccurred())

				By("watching for changes without a starting revision")
				updateC3, err := ts.Watch(ctx, "key")

				By("ensuring that the first watcher sees all events")
				matchEvent(updateC1, storage.WatchEventPut, "key", none, noRev, newT(1), revisions[0])
				matchEvent(updateC1, storage.WatchEventPut, "key", newT(1), revisions[0], newT(2), revisions[1])

				By("ensuring that the second watcher only sees the second event")
				matchEvent(updateC2, storage.WatchEventPut, "key", newT(1), revisions[0], newT(2), revisions[1])

				By("ensuring that the third watcher receives nothing")
				Consistently(updateC3).WithTimeout(10 * time.Millisecond).ShouldNot(Receive())

				By("updating the key again")
				Expect(ts.Put(context.Background(), "key", newT(3), storage.WithRevisionOut(&revisions[2]))).To(Succeed())

				By("ensuring that all watchers see the update")
				matchEvent(updateC1, storage.WatchEventPut, "key", newT(2), revisions[1], newT(3), revisions[2])
				matchEvent(updateC2, storage.WatchEventPut, "key", newT(2), revisions[1], newT(3), revisions[2])
				matchEvent(updateC3, storage.WatchEventPut, "key", newT(2), revisions[1], newT(3), revisions[2])
			})

			It("should stop watching when the context is canceled", func(ctx SpecContext) {
				wctx, cancel := context.WithCancel(ctx)
				updateC1, err := ts.Watch(wctx, "key")
				Expect(err).NotTo(HaveOccurred())
				updateC2, err := ts.Watch(ctx, "key")
				Expect(err).NotTo(HaveOccurred())

				var revisions [2]int64
				Expect(ts.Put(ctx, "key", newT(1), storage.WithRevisionOut(&revisions[0]))).To(Succeed())
				matchEvent(updateC1, storage.WatchEventPut, "key", none, noRev, newT(1), revisions[0])
				matchEvent(updateC2, storage.WatchEventPut, "key", none, noRev, newT(1), revisions[0])
				cancel()
				Expect(ts.Put(ctx, "key", newT(2), storage.WithRevisionOut(&revisions[1]))).To(Succeed())
				Eventually(updateC1).Should(BeClosed())
				matchEvent(updateC2, storage.WatchEventPut, "key", newT(1), revisions[0], newT(2), revisions[1])
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
					Expect(value).To(match(newT(2)))

					// Check with correct revision
					Expect(ts.Put(context.Background(), "key1", newT(3), storage.WithRevision(revision2))).To(Succeed())

					value, err = ts.Get(context.Background(), "key1")
					Expect(err).NotTo(HaveOccurred())
					Expect(value).To(match(newT(3)))
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
						Expect(value).To(match(newT(int64(i))))
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
							Expect(v).To(match(newT(1)))
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
						Expect(value).To(match(newT(2)))
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
					Expect(value).To(match(newT(2)))

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
				It("should store key history", SpecTimeout(1*time.Minute), func(ctx context.Context) {
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
									Expect(revs[k].Value()).To(match(newT(int64(k))))
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
				When("no revision is specified", func() {
					It("should return the history up to the current revision", func() {
						Expect(ts.Put(context.Background(), "key1", newT(1))).To(Succeed())
						Expect(ts.Put(context.Background(), "key1", newT(2))).To(Succeed())
						Expect(ts.Put(context.Background(), "key1", newT(3))).To(Succeed())

						revs, err := ts.History(context.Background(), "key1", storage.IncludeValues(true))
						Expect(err).NotTo(HaveOccurred())
						Expect(revs).To(HaveLen(3))

						Expect(revs[0].Key()).To(Equal("key1"))
						Expect(revs[0].Value()).To(match(newT(1)))
						Expect(revs[0].Revision()).To(BeNumerically(">", 0))

						Expect(revs[1].Key()).To(Equal("key1"))
						Expect(revs[1].Value()).To(match(newT(2)))
						Expect(revs[1].Revision()).To(BeNumerically(">", revs[0].Revision()))

						Expect(revs[2].Key()).To(Equal("key1"))
						Expect(revs[2].Value()).To(match(newT(3)))
						Expect(revs[2].Revision()).To(BeNumerically(">", revs[1].Revision()))
					})
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
						Expect(revs[0].Value()).To(match(newT(1)))
						Expect(revs[1].Value()).To(match(newT(2)))
						Expect(revs[2].Value()).To(match(newT(3)))
					})
				})
			})
		})
	}
}
