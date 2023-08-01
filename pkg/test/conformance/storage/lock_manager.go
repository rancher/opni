package conformance_storage

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/samber/lo"
)

func LockManagerTestSuite[T storage.LockManager](
	lmsF future.Future[[]T],

) func() {
	return func() {
		var lm T
		var lm2 T
		BeforeAll(func() {
			arr := lmsF.Get()
			lm = arr[0]
			lm2 = arr[1]
		})

		BeforeEach(func() {
			jetstream.GlobalLockId = 0
		})

		When("using distributed locks ", func() {
			It("should only request lock actions once", func() {
				lock1 := lm.Locker("foo", lock.WithExclusiveLock())
				err := lock1.Lock()
				Expect(err).NotTo(HaveOccurred())
				err = lock1.Lock()
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(lock.ErrLockActionRequested))

				err = lock1.Unlock()
				Expect(err).NotTo(HaveOccurred())
				err = lock1.Unlock()
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(lock.ErrLockActionRequested))
			})
			It("should lock and unlock locks of the same type", func() {
				lock1 := lm.Locker("todo")
				Expect(lock1.Lock()).To(Succeed())
				Expect(lock1.Unlock()).To(Succeed())

				lock2 := lm.Locker("todo")
				Expect(lock2.Lock()).To(Succeed())
				Expect(lock2.Unlock()).To(Succeed())
			})
			It("should resolve concurrent lock conflicts", func() {
				locks := []LockWithTransaction{}
				for i := 0; i < 2; i++ {
					lock := lm.Locker("todo")
					locks = append(locks, LockWithTransaction{
						A: lock,
						C: make(chan struct{}),
					})
				}
				errs := make(chan error, 2*len(locks))

				lockOrder := lo.Shuffle(locks)
				var wg sync.WaitGroup
				for _, lock := range lockOrder {
					lock := lock
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := lock.A.Lock()
						sendWithJitter(lock.C)
						errs <- err
					}()
				}
				for _, lock := range lockOrder {
					lock := lock
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-lock.C
						errs <- lock.A.Unlock()
					}()
				}
				wg.Wait()
				Eventually(errs).Should(Receive(BeNil()))
			})

			// XSpecify("by using invalid lock keys", func() {
			// 	invalidKey := ""
			// 	lock1 := lm.Locker(invalidKey, lock.WithExclusiveLock())
			// 	err := lock1.Lock()
			// 	Expect(err).To(HaveOccurred())
			// 	err = lock1.Unlock()
			// 	Expect(err).To(HaveOccurred())

			// 	lock2 := lm.Locker(invalidKey, lock.WithExclusiveLock())
			// 	err = lock2.Lock()
			// 	Expect(err).To(HaveOccurred())
			// 	err = lock2.Unlock()
			// 	Expect(err).To(HaveOccurred())
			// })

			Specify("locks should be able to acquire the lock if the existing lock has expired", func() {
				exLock := lm.Locker("bar", lock.WithRetryDelay(1*time.Millisecond), lock.WithExpireDuration(time.Millisecond*50))
				otherLock := lm.Locker("bar", lock.WithMaxRetries(10), lock.WithAcquireTimeout(time.Second))
				err := exLock.Lock()
				Expect(err).NotTo(HaveOccurred())

				time.Sleep(2 * time.Second)

				err = otherLock.Lock()
				Expect(err).NotTo(HaveOccurred())

				By("releasing the expired exclusive lock should not affect releasing newer locks")
				err = exLock.Unlock()
				Expect(err).NotTo(HaveOccurred())

				err = otherLock.Unlock()
				Expect(err).NotTo(HaveOccurred())
			})

			Specify("any lock should timeout after the specified timeout", func() {
				Expect(ExpectTimeout(lm, lock.EX)).To(Succeed())
				Expect(ExpectTimeout(lm, lock.PW)).To(Succeed())
				Expect(ExpectTimeout(lm, lock.PR)).To(Succeed())
				Expect(ExpectTimeout(lm, lock.CW)).To(Succeed())
				Expect(ExpectTimeout(lm, lock.CR)).To(Succeed())

			})

			Specify("lock should respect their context errors", func() {
				Expect(ExpectCancellable(lm, lock.EX)).To(Succeed())
				Expect(ExpectCancellable(lm, lock.PW)).To(Succeed())
				Expect(ExpectCancellable(lm, lock.PR)).To(Succeed())
				Expect(ExpectCancellable(lm, lock.CW)).To(Succeed())
				Expect(ExpectCancellable(lm, lock.CR)).To(Succeed())
			})

			// Specify("locks should respect their retry limits", func() {
			// 	Expect(ExpectRetryLimit(lm, lock.EX)).To(Succeed())
			// 	Expect(ExpectRetryLimit(lm, lock.PW)).To(Succeed())
			// 	Expect(ExpectRetryLimit(lm, lock.PR)).To(Succeed())
			// 	Expect(ExpectRetryLimit(lm, lock.CW)).To(Succeed())
			// 	Expect(ExpectRetryLimit(lm, lock.CR)).To(Succeed())
			// })
		})

		When("using exclusive locks", func() {
			It("should allow multiple writers to safely write using locks", func() {
				lock1 := lm.Locker("foo")
				err := ExpectAtomic2(lock1, func(opts ...lock.LockOption) storage.Lock {
					return lm.Locker("foo", opts...)
				})
				Expect(err).NotTo(HaveOccurred())
			})
			// It("should prevent protected writes from accessing anything held by the exclusive lock", func() {
			// 	err := ExpectLockToBlock(lm, lock.EX, lock.PW)
			// 	Expect(err).NotTo(HaveOccurred())
			// })
			// It("should prevent protected reads from accessing anything held by the exclusive lock", func() {
			// 	err := ExpectLockToBlock(lm, lock.EX, lock.PR)
			// 	Expect(err).NotTo(HaveOccurred())
			// })
			// It("should prevent concurrent writes from accessing anything held by the exclusive lock", func() {
			// 	err := ExpectLockToBlock(lm, lock.EX, lock.CW)
			// 	Expect(err).NotTo(HaveOccurred())
			// })
			// It("should prevent concurrent reads from accessing anything held by the exclusive lock", func() {
			// 	err := ExpectLockToBlock(lm, lock.EX, lock.CR)
			// 	Expect(err).NotTo(HaveOccurred())
			// })
		})

		When("using RW locks", func() {
			Specify("write locks should be exclusive", func() {
				// // TODO : rework this test
				// err := ExpectAtomic(lm, lock.PW)
				// Expect(err).NotTo(HaveOccurred())
			})

			Specify("write & exclusive locks should be exclusive", func() {
				// // TODO : rework this test
				// err := ExpectAtomic(lm, lock.EX)
				// Expect(err).NotTo(HaveOccurred())
			})

			// It("should allow concurrent readers to read while some of the protected writers write", func() {
			// 	err := ExpectLockNotToBlock(lm, lock.PW, lock.CR)
			// 	Expect(err).NotTo(HaveOccurred())
			// })
			// It("should allow concurrent writes", func() {
			// 	err := ExpectLockNotToBlock(lm, lock.PW, lock.CR)
			// 	Expect(err).NotTo(HaveOccurred())
			// })
			// It("should block exclusive acquisitions", func() {
			// 	err := ExpectLockToBlock(lm, lock.PW, lock.EX)
			// 	Expect(err).NotTo(HaveOccurred())
			// })
		})

		// When("using protected read locks", func() {
		// 	It("should allow multiple protected readers to concurrently read", func() {
		// 		err := ExpectLockNotToBlock(lm, lock.PR, lock.PR)
		// 		Expect(err).NotTo(HaveOccurred())
		// 	})
		// 	It("should allow multiple concurrent reads to concurrently read during a protected read", func() {
		// 		err := ExpectLockNotToBlock(lm, lock.PR, lock.CR)
		// 		Expect(err).NotTo(HaveOccurred())
		// 	})
		// 	It("should block any writes from happening", func() {
		// 		writeAccessTypes := []lock.AccessType{lock.CW, lock.PW, lock.EX}
		// 		for _, at := range writeAccessTypes {
		// 			err := ExpectLockToBlock(lm, lock.PR, at)
		// 			Expect(err).NotTo(HaveOccurred())
		// 		}
		// 	})

		// })

		// When("using concurrent write locks", func() {
		// 	It("should allow concurrent writes to follow any order", func() {
		// 		err := ExpectLockNotToBlock(lm, lock.CW, lock.CW)
		// 		Expect(err).NotTo(HaveOccurred())
		// 	})
		// 	It("should not block concurrent reads from happening", func() {
		// 		err := ExpectLockNotToBlock(lm, lock.CW, lock.CR)
		// 		Expect(err).NotTo(HaveOccurred())
		// 	})
		// 	It("should block other write types from happening", func() {
		// 		incompatibleWriteTypes := []lock.AccessType{lock.PW, lock.EX, lock.PR}
		// 		for _, at := range incompatibleWriteTypes {
		// 			err := ExpectLockToBlock(lm, lock.CW, at)
		// 			Expect(err).NotTo(HaveOccurred())
		// 		}
		// 	})
		// })

		XWhen("using multiple lock clients", func() {
			It("should prevent undefined behaviour by inadvertently releasing locks early", func() {
				lockPW := lm.Locker("foo", lock.WithProtectedWrite())
				lockCR := lm.Locker("foo", lock.WithConcurrentRead())

				lockPR := lm.Locker("foo", lock.WithProtectedRead()) // this should not be able to access lockPW

				err := lockPW.Lock()
				Expect(err).NotTo(HaveOccurred())
				err = lockCR.Lock()
				Expect(err).NotTo(HaveOccurred())

				err = lockCR.Unlock()
				Expect(err).NotTo(HaveOccurred())

				By("preventing inadvertent access to locks that are exclusive to each other")

				err = lockPR.Lock()
				Expect(err).To(HaveOccurred())

				By("verifying once the offending lock is released we can acquire a new lock")

				err = lockPW.Unlock()
				Expect(err).NotTo(HaveOccurred())

				lockPR2 := lm.Locker("foo", lock.WithProtectedRead())
				err = lockPR2.Lock()
				Expect(err).NotTo(HaveOccurred())

				err = lockPR2.Unlock()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should prevent undefined behaviour by inadvertently releasing locks early when using multiple clients", func() {

				By("ensuring one of the clients acquire a high privilege lock")
				lockPW := lm.Locker("foo", lock.WithProtectedWrite())

				err := lockPW.Lock()
				Expect(err).NotTo(HaveOccurred())

				By("having both clients acquire & release low privilege locks that bypass the other lock")
				lockCR1 := lm.Locker("foo", lock.WithConcurrentRead())
				lockCR2 := lm2.Locker("foo", lock.WithConcurrentRead())
				err = lockCR1.Lock()
				Expect(err).NotTo(HaveOccurred())
				err = lockCR2.Lock()
				Expect(err).NotTo(HaveOccurred())

				err = lockCR2.Unlock()
				Expect(err).NotTo(HaveOccurred())
				err = lockCR1.Unlock()
				Expect(err).NotTo(HaveOccurred())

				By("making sure neither client can acquire a lock that is blocked by the high privlege lock")

				lockPR1 := lm.Locker("foo", lock.WithProtectedRead())
				lockPR2 := lm2.Locker("foo", lock.WithProtectedRead())

				err = lockPR1.Lock()
				Expect(err).To(HaveOccurred())

				err = lockPR2.Lock()
				Expect(err).To(HaveOccurred())

				By("making sure the high privilege lock can be released")
				err = lockPW.Unlock()
				Expect(err).NotTo(HaveOccurred())

				By("making sure the low privilege locks can be acquired & released now that the high privilege lock is released")
				lockPR1 = lm.Locker("foo", lock.WithProtectedRead())
				lockPR2 = lm2.Locker("foo", lock.WithProtectedRead())

				err = lockPR1.Lock()
				Expect(err).NotTo(HaveOccurred())
				err = lockPR2.Lock()
				Expect(err).NotTo(HaveOccurred())

				err = lockPR1.Unlock()
				Expect(err).NotTo(HaveOccurred())
				err = lockPR2.Unlock()
				Expect(err).NotTo(HaveOccurred())
			})
		})
	}
}
