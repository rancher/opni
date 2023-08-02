package conformance_storage

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

func ExpectTimeout(lm storage.LockManager, a lock.AccessType) error {
	lock1 := lm.Locker("bar", lock.WithExclusiveLock(), lock.WithAcquireTimeout(time.Second), lock.WithExpireDuration(5*time.Second))
	lock2 := lm.Locker("bar", lock.WithAccessType(a))

	err := lock1.Lock()
	if err != nil {
		return err
	}

	err = lock2.Lock()
	if !errors.Is(err, lock.ErrAcquireLockTimeout) {
		return fmt.Errorf("expected lock to timeout, but got %v", err)
	}

	err = lock2.Unlock()
	if !errors.Is(err, lock.ErrLockNotAcquired) {
		return fmt.Errorf("expected unlock to fail, but got %v", err)
	}

	return lock1.Unlock()
}

func ExpectRetryLimit(lm storage.LockManager, a lock.AccessType) error {
	lock1 := lm.Locker("bar", lock.WithExclusiveLock(), lock.WithAcquireTimeout(time.Second), lock.WithExpireDuration(5*time.Second))
	lock2 := lm.Locker("bar", lock.WithAccessType(a), lock.WithMaxRetries(1))

	err := lock1.Lock()
	if err != nil {
		return err
	}

	err = lock2.Lock()
	if !errors.Is(err, lock.ErrAcquireLockRetryExceeded) {
		return fmt.Errorf("expected lock to timeout, but got %v", err)
	}

	err = lock2.Unlock()
	if !errors.Is(err, lock.ErrLockNotAcquired) {
		return fmt.Errorf("expected unlock to fail, but got %v", err)
	}

	return lock1.Unlock()
}

func ExpectCancellable(lm storage.LockManager, a lock.AccessType) error {
	ctxca, cancel := context.WithCancel(context.Background())
	defer cancel()
	exLock := lm.Locker("bar", lock.WithExclusiveLock(), lock.WithExpireDuration(time.Hour))
	cancelLock := lm.Locker("bar", lock.WithAccessType(a), lock.WithContext(ctxca), lock.WithRetryDelay(time.Microsecond*500))
	err := exLock.Lock()
	if err != nil {
		return err
	}
	go func() {
		time.Sleep(500 * time.Microsecond)
		cancel()
	}()
	err = cancelLock.Lock()
	if !errors.Is(err, lock.ErrAcquireLockCancelled) {
		return fmt.Errorf("expected lock to be cancelled, but got %v", err)
	}

	err = cancelLock.Unlock()
	if !errors.Is(err, lock.ErrLockNotAcquired) {
		return fmt.Errorf("expected unlock to fail, but got %v", err)
	}
	return exLock.Unlock()

}

var accessTypes = []lock.AccessType{lock.EX, lock.PR, lock.PW, lock.CW, lock.CR}

func LockManagerTestSuiteOld[T storage.LockManager](
	lms future.Future[[]T],
) func() {
	return func() {
		var lm T
		var lm2 T
		BeforeAll(func() {
			lms := lms.Get()
			lm = lms[0]
			lm2 = lms[1]
		})

		BeforeEach(func() {
			jetstream.GlobalLockId = 0
		})

		When("using the lock manager", func() {
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
				for _, accessType := range accessTypes {
					By(fmt.Sprintf("verifying we can lock and unlock locks of type %d", accessType))
					lock1 := lm.Locker("todo", lock.WithAccessType(accessType))
					Expect(lock1.Lock()).To(Succeed())
					Expect(lock1.Unlock()).To(Succeed())

					By(fmt.Sprintf("checking that lock resources are adequately freed : %d", accessType))
					lock2 := lm.Locker("todo", lock.WithAccessType(accessType))
					Expect(lock2.Lock()).To(Succeed())
					Expect(lock2.Unlock()).To(Succeed())
				}
			})
			It("should resolve concurrent lock conflicts of the same type", func() {
				for _, accessType := range accessTypes {
					By(fmt.Sprintf("for access type : %d", accessType))
					locks := []LockWithTransaction{}
					for i := 0; i < 2; i++ {
						lock := lm.Locker("todo", lock.WithAccessType(accessType))
						locks = append(locks, LockWithTransaction{
							A: lock,
							B: NewTransaction(accessType),
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
				}
			})

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

		When("we encounter lock failures", func() {
			Specify("by using invalid lock keys", func() {
				invalidKey := ""
				lock1 := lm.Locker(invalidKey, lock.WithExclusiveLock())
				err := lock1.Lock()
				Expect(err).To(HaveOccurred())
				err = lock1.Unlock()
				Expect(err).To(HaveOccurred())

				lock2 := lm.Locker(invalidKey, lock.WithExclusiveLock())
				err = lock2.Lock()
				Expect(err).To(HaveOccurred())
				err = lock2.Unlock()
				Expect(err).To(HaveOccurred())
			})

			Specify("any other lock should be able to acquire the lock if the existing lock has expired", func() {
				for _, lockType := range accessTypes {
					By(fmt.Sprintf("lock type %s should be able to acquire expired locks", lock.AccessTypeToString(lockType)))
					exLock := lm.Locker("bar", lock.WithExclusiveLock(), lock.WithRetryDelay(1*time.Millisecond), lock.WithAcquireTimeout(time.Second), lock.WithExpireDuration(time.Millisecond*50))
					otherLock := lm.Locker("bar", lock.WithAccessType(lockType), lock.WithMaxRetries(10), lock.WithAcquireTimeout(100*time.Millisecond))
					err := exLock.Lock()
					Expect(err).NotTo(HaveOccurred())

					time.Sleep(60 * time.Millisecond)

					err = otherLock.Lock()
					Expect(err).NotTo(HaveOccurred())

					By("releasing the expired exclusive lock should not affect releasing newer locks")
					err = exLock.Unlock()
					Expect(err).NotTo(HaveOccurred())

					err = otherLock.Unlock()
					Expect(err).NotTo(HaveOccurred())
				}
			})

			Specify("any lock should timeout after the specified timeout", func() {
				frozen := lock.DefaultAcquireTimeout
				lock.DefaultAcquireTimeout = 50 * time.Millisecond
				Expect(ExpectTimeout(lm, lock.EX)).To(Succeed())
				Expect(ExpectTimeout(lm, lock.PW)).To(Succeed())
				Expect(ExpectTimeout(lm, lock.PR)).To(Succeed())
				Expect(ExpectTimeout(lm, lock.CW)).To(Succeed())
				Expect(ExpectTimeout(lm, lock.CR)).To(Succeed())
				lock.DefaultAcquireTimeout = frozen
			})

			Specify("lock should respect their context errors", func() {
				Expect(ExpectCancellable(lm, lock.EX)).To(Succeed())
				Expect(ExpectCancellable(lm, lock.PW)).To(Succeed())
				Expect(ExpectCancellable(lm, lock.PR)).To(Succeed())
				Expect(ExpectCancellable(lm, lock.CW)).To(Succeed())
				Expect(ExpectCancellable(lm, lock.CR)).To(Succeed())
			})

			Specify("locks should respect their retry limits", func() {
				frozen := lock.DefaultMaxRetries
				lock.DefaultMaxRetries = 3
				Expect(ExpectRetryLimit(lm, lock.EX)).To(Succeed())
				Expect(ExpectRetryLimit(lm, lock.PW)).To(Succeed())
				Expect(ExpectRetryLimit(lm, lock.PR)).To(Succeed())
				Expect(ExpectRetryLimit(lm, lock.CW)).To(Succeed())
				Expect(ExpectRetryLimit(lm, lock.CR)).To(Succeed())
				lock.DefaultMaxRetries = frozen
			})
		})

		When("using exclusive locks", func() {
			It("should allow multiple writers to safely write using locks", func() {
				err := ExpectAtomic(lm, lock.EX)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should prevent protected writes from accessing anything held by the exclusive lock", func() {
				err := ExpectLockToBlock(lm, lock.EX, lock.PW)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should prevent protected reads from accessing anything held by the exclusive lock", func() {
				err := ExpectLockToBlock(lm, lock.EX, lock.PR)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should prevent concurrent writes from accessing anything held by the exclusive lock", func() {
				err := ExpectLockToBlock(lm, lock.EX, lock.CW)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should prevent concurrent reads from accessing anything held by the exclusive lock", func() {
				err := ExpectLockToBlock(lm, lock.EX, lock.CR)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("using protected write locks", func() {
			It("should allow multiple writers to safely write using locks", func() {
				err := ExpectAtomic(lm, lock.PW)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should allow concurrent readers to read while some of the protected writers write", func() {
				err := ExpectLockNotToBlock(lm, lock.PW, lock.CR)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should allow concurrent writes", func() {
				err := ExpectLockNotToBlock(lm, lock.PW, lock.CR)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should block exclusive acquisitions", func() {
				err := ExpectLockToBlock(lm, lock.PW, lock.EX)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("using protected read locks", func() {
			It("should allow multiple protected readers to concurrently read", func() {
				err := ExpectLockNotToBlock(lm, lock.PR, lock.PR)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should allow multiple concurrent reads to concurrently read during a protected read", func() {
				err := ExpectLockNotToBlock(lm, lock.PR, lock.CR)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should block any writes from happening", func() {
				writeAccessTypes := []lock.AccessType{lock.CW, lock.PW, lock.EX}
				for _, at := range writeAccessTypes {
					err := ExpectLockToBlock(lm, lock.PR, at)
					Expect(err).NotTo(HaveOccurred())
				}
			})

		})

		When("using concurrent write locks", func() {
			It("should allow concurrent writes to follow any order", func() {
				err := ExpectLockNotToBlock(lm, lock.CW, lock.CW)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should not block concurrent reads from happening", func() {
				err := ExpectLockNotToBlock(lm, lock.CW, lock.CR)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should block other write types from happening", func() {
				incompatibleWriteTypes := []lock.AccessType{lock.PW, lock.EX, lock.PR}
				for _, at := range incompatibleWriteTypes {
					err := ExpectLockToBlock(lm, lock.CW, at)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})
	}
}

type LockWithTransaction lo.Tuple3[storage.Lock, *Transaction, chan struct{}]

func ExpectLockToBlock(lm storage.LockManager, blockAccess, otherAccess lock.AccessType) error {
	lock1 := lm.Locker("foo", lock.WithAccessType(blockAccess))
	err := lock1.Lock()
	Expect(err).NotTo(HaveOccurred())

	otherLocks := []storage.Lock{
		lm.Locker("foo", lock.WithAccessType(otherAccess), lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
		lm.Locker("foo", lock.WithAccessType(otherAccess), lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
		lm.Locker("foo", lock.WithAccessType(otherAccess), lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
	}

	var numLocksAcquired uint32
	var numLocksTimeouted uint32

	var blockerEg errgroup.Group
	startUnlock := make(chan error)
	var blockedEg errgroup.Group
	go func() {
		for _, olock := range otherLocks {
			olock := olock
			blockedEg.Go(func() error {
				err := olock.Lock()
				if err == nil {
					atomic.AddUint32(&numLocksAcquired, 1)
					return fmt.Errorf("expected lock to be blocked, but it was acquired")
				}
				if errors.Is(err, lock.ErrAcquireLockTimeout) || errors.Is(err, lock.ErrAcquireLockRetryExceeded) {
					atomic.AddUint32(&numLocksTimeouted, 1)
				}
				return nil
			})
		}
		err := blockedEg.Wait()
		startUnlock <- err

	}()

	blockerEg.Go(func() error {
		err := <-startUnlock
		if err != nil {
			return err
		}
		return lock1.Unlock()
	})
	if err := blockerEg.Wait(); err != nil {
		return err
	}
	if numLocksAcquired > 0 {
		return fmt.Errorf("expected no locks to be acquired, but got %d", numLocksAcquired)
	}
	if numLocksTimeouted != uint32(len(otherLocks)) {
		return fmt.Errorf("expected %d locks to timeout, but got %d", len(otherLocks), numLocksTimeouted)
	}
	return nil
}

func ExpectLockNotToBlock(lm storage.LockManager, firstType, otherType lock.AccessType) error {
	mainLock := lm.Locker("foo", lock.WithAccessType(firstType))

	otherLocks := []storage.Lock{
		lm.Locker("foo", lock.WithAccessType(otherType)),
		lm.Locker("foo", lock.WithAccessType(otherType)),
		lm.Locker("foo", lock.WithAccessType(otherType)),
	}

	if err := mainLock.Lock(); err != nil {
		return err
	}

	lockOrder := lo.Shuffle(otherLocks)
	for _, lock := range lockOrder {
		if err := lock.Lock(); err != nil {
			return err
		}
	}

	unlockOrder := lo.Shuffle(otherLocks)
	for _, lock := range unlockOrder {
		if err := lock.Unlock(); err != nil {
			return err
		}
	}
	if err := mainLock.Unlock(); err != nil {
		return err
	}
	return nil
}

func ExpectAtomic2(lock1 storage.Lock, lockConstructor func(opts ...lock.LockOption) storage.Lock) error {
	err := lock1.Lock()
	Expect(err).NotTo(HaveOccurred())

	otherLocks := []storage.Lock{
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
	}

	var numLocksAcquired uint32
	var numLocksTimeouted uint32

	var wg sync.WaitGroup
	var toUnlock storage.Lock

	wg.Add(2)

	go func() {
		var nestedWg sync.WaitGroup
		defer wg.Done()
		defer GinkgoRecover()
		nestedWg.Add(len(otherLocks))
		for _, olock := range otherLocks {
			olock := olock
			go func() {
				defer nestedWg.Done()
				err := olock.Lock()
				if err == nil {
					atomic.AddUint32(&numLocksAcquired, 1)
					toUnlock = olock
				}
				if errors.Is(err, lock.ErrAcquireLockTimeout) {
					atomic.AddUint32(&numLocksTimeouted, 1)
				}
			}()
		}
		nestedWg.Wait()

	}()

	go func() {
		<-time.After(20 * time.Millisecond)
		defer GinkgoRecover()
		err := lock1.Unlock()
		Expect(err).NotTo(HaveOccurred())
		defer wg.Done()
	}()

	wg.Wait()
	if toUnlock == nil {
		return fmt.Errorf("expected one of the locks to be acquired, but none were (%d timeouted)", numLocksTimeouted)
	}
	if err := toUnlock.Unlock(); err != nil {
		return err
	}
	Expect(err).NotTo(HaveOccurred())
	if numLocksAcquired != 1 {
		return fmt.Errorf("expected only one lock to be acquired, but got %d", numLocksAcquired)
	}
	if numLocksTimeouted != uint32(len(otherLocks)-1) {
		return fmt.Errorf("expected %d locks to timeout, but got %d", len(otherLocks)-1, numLocksTimeouted)
	}
	return nil
}

// Sequential execution is guaranteed by :
// - first starting a lock
// - having multiple candidate locks try and concurrently acquire the lock
// - when the first lock is released, only one of the candidate locks should be able to acquire the lock
func ExpectAtomic(lm storage.LockManager, accessType lock.AccessType) error {
	lock1 := lm.Locker("foo", lock.WithAccessType(accessType))
	err := lock1.Lock()
	Expect(err).NotTo(HaveOccurred())

	otherLocks := []storage.Lock{
		lm.Locker("foo", lock.WithAccessType(accessType), lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(100*time.Millisecond)),
		lm.Locker("foo", lock.WithAccessType(accessType), lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(100*time.Millisecond)),
		lm.Locker("foo", lock.WithAccessType(accessType), lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(100*time.Millisecond)),
	}

	var numLocksAcquired uint32
	var numLocksTimeouted uint32

	var wg sync.WaitGroup
	var toUnlock storage.Lock

	wg.Add(2)

	go func() {
		var nestedWg sync.WaitGroup
		defer wg.Done()
		defer GinkgoRecover()
		nestedWg.Add(len(otherLocks))
		for _, olock := range otherLocks {
			olock := olock
			go func() {
				defer nestedWg.Done()
				err := olock.Lock()
				if err == nil {
					atomic.AddUint32(&numLocksAcquired, 1)
					toUnlock = olock
				}
				if errors.Is(err, lock.ErrAcquireLockTimeout) || errors.Is(err, lock.ErrAcquireLockRetryExceeded) {
					atomic.AddUint32(&numLocksTimeouted, 1)
				}
			}()
		}
		nestedWg.Wait()

	}()

	go func() {
		<-time.After(20 * time.Millisecond)
		defer GinkgoRecover()
		err := lock1.Unlock()
		Expect(err).NotTo(HaveOccurred())
		defer wg.Done()
	}()

	wg.Wait()
	if toUnlock == nil {
		return fmt.Errorf("expected one of the locks to be acquired, but none were")
	}
	if err := toUnlock.Unlock(); err != nil {
		return err
	}
	Expect(err).NotTo(HaveOccurred())
	if numLocksAcquired != 1 {
		return fmt.Errorf("expected only one lock to be acquired, but got %d", numLocksAcquired)
	}
	if numLocksTimeouted != uint32(len(otherLocks)-1) {
		return fmt.Errorf("expected %d locks to timeout, but got %d", len(otherLocks)-1, numLocksTimeouted)
	}
	return nil
}

type Transaction struct {
	Uuid string
	lock.AccessType
}

type RecordedTransaction struct {
	*Transaction
	Lock   bool
	Unlock bool
}

func NewTransaction(accessType lock.AccessType) *Transaction {
	return &Transaction{
		Uuid:       uuid.New().String(),
		AccessType: accessType,
	}
}

func jitter() time.Duration {
	rand.Seed(time.Now().UnixNano())
	randomMicroseconds := rand.Intn(101)
	jitterInterval := time.Duration(randomMicroseconds) * time.Microsecond
	return jitterInterval
}

func sendWithJitter(send chan struct{}) {
	<-time.After(jitter())
	send <- struct{}{}
}
