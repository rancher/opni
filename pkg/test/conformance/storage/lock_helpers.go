package conformance_storage

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

// Timeouts are only valid if we specified lock.Keeplive(false)
func ExpectTimeout(lm storage.LockManager) error {
	lock1 := lm.Locker("bar", lock.WithKeepalive(false), lock.WithAcquireTimeout(time.Second), lock.WithExpireDuration(5*time.Second))
	lock2 := lm.Locker("bar")

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

func ExpectCancellable(lm storage.LockManager) error {
	ctxca, cancel := context.WithCancel(context.Background())
	defer cancel()
	exLock := lm.Locker("bar", lock.WithExpireDuration(time.Hour))
	cancelLock := lm.Locker("bar", lock.WithContext(ctxca), lock.WithRetryDelay(time.Microsecond*500))
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

type LockWithTransaction lo.Tuple3[storage.Lock, *Transaction, chan struct{}]

func ExpectLockToBlock(lm storage.LockManager, blockAccess, otherAccess lock.AccessType) error {
	lock1 := lm.Locker("foo")
	err := lock1.Lock()
	Expect(err).NotTo(HaveOccurred())

	otherLocks := []storage.Lock{
		lm.Locker("foo", lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
		lm.Locker("foo", lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
		lm.Locker("foo", lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
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
	mainLock := lm.Locker("foo")

	otherLocks := []storage.Lock{
		lm.Locker("foo"),
		lm.Locker("foo"),
		lm.Locker("foo"),
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

// Sequential execution is guaranteed by :
// - first starting a lock
// - having multiple candidate locks try and concurrently acquire the lock
// - when the first lock is released, only one of the candidate locks should be able to acquire the lock
func ExpectAtomic(lock1 storage.Lock, lockConstructor func(opts ...lock.LockOption) storage.Lock) error {
	err := lock1.Lock()
	Expect(err).NotTo(HaveOccurred())

	otherLocks := []storage.Lock{
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
	}

	var numLocksAcquired uint32
	var numLocksTimeouted uint32
	var tasksSpawned uint32
	expectedTasks := len(otherLocks)

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
				atomic.AddUint32(&tasksSpawned, 1)
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
		for {
			tasks := atomic.LoadUint32(&tasksSpawned)
			if tasks == uint32(expectedTasks) {
				break
			}
		}
		defer GinkgoRecover()
		lock1.Unlock()
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

func ExpectAtomicKeepAlive(lock1 storage.Lock, lockConstructor func(opts ...lock.LockOption) storage.Lock) error {
	err := lock1.Lock()
	Expect(err).NotTo(HaveOccurred())

	otherLocks := []storage.Lock{
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(50*time.Millisecond)),
	}

	var numLocksAcquired uint32
	var numLocksTimeouted uint32
	var tasksSpawned uint32
	expectedTasks := len(otherLocks)

	var wg sync.WaitGroup

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
				atomic.AddUint32(&tasksSpawned, 1)
				err := olock.Lock()
				if err == nil {
					atomic.AddUint32(&numLocksAcquired, 1)
				}
				if errors.Is(err, lock.ErrAcquireLockTimeout) {
					atomic.AddUint32(&numLocksTimeouted, 1)
				}
			}()
		}
		nestedWg.Wait()

	}()

	go func() {
		for {
			tasks := atomic.LoadUint32(&tasksSpawned)
			if tasks == uint32(expectedTasks) {
				break
			}
		}
		defer wg.Done()
	}()

	wg.Wait()
	Expect(err).NotTo(HaveOccurred())
	if numLocksAcquired != 0 {
		return fmt.Errorf("expected no locks to be acquired, but got %d", numLocksAcquired)
	}
	if numLocksTimeouted != uint32(len(otherLocks)) {
		return fmt.Errorf("expected all %d locks to timeout, but got %d", len(otherLocks), numLocksTimeouted)
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
