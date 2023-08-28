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

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/samber/lo"
)

// Timeouts are only valid if we specified lock.Keeplive(false)
func expectTimeout(lm storage.LockManager) error {
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

func expectCancellable(lm storage.LockManager) error {
	ctxca, cancel := context.WithCancel(context.Background())
	defer cancel()
	exLock := lm.Locker("bar", lock.WithExpireDuration(time.Hour))
	cancelLock := lm.Locker("bar", lock.WithAcquireContext(ctxca), lock.WithRetryDelay(time.Microsecond*500))
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

type lockWithTransaction lo.Tuple3[storage.Lock, *transaction, chan struct{}]

// Sequential execution is guaranteed by :
// - first starting a lock
// - having multiple candidate locks try and concurrently acquire the lock
// - when the first lock is released, only one of the candidate locks should be able to acquire the lock
func expectAtomic(lock1 storage.Lock, lockConstructor func(opts ...lock.LockOption) storage.Lock) error {
	err := lock1.Lock()
	Expect(err).NotTo(HaveOccurred())

	otherLocks := []storage.Lock{
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(150*time.Millisecond)),
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(150*time.Millisecond)),
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(150*time.Millisecond)),
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
		defer wg.Done()
		Expect(err).NotTo(HaveOccurred())
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

func expectAtomicKeepAlive(lock1 storage.Lock, lockConstructor func(opts ...lock.LockOption) storage.Lock) error {
	err := lock1.Lock()
	Expect(err).NotTo(HaveOccurred())

	otherLocks := []storage.Lock{
		// some implementations expire durations are forced to round up to the largest second, so 2 *time.Second is a requirement here
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(2*time.Second)),
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(2*time.Second)),
		lockConstructor(lock.WithExpireDuration(time.Second), lock.WithAcquireTimeout(2*time.Second)),
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
		defer wg.Done()
		for {
			tasks := atomic.LoadUint32(&tasksSpawned)
			if tasks == uint32(expectedTasks) {
				break
			}
		}
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

type transaction struct {
	Uuid string
}

func jitter() time.Duration {
	rand.NewSource(time.Now().UnixNano())
	randomMicroseconds := rand.Intn(101)
	jitterInterval := time.Duration(randomMicroseconds) * time.Microsecond
	return jitterInterval
}

func sendWithJitter(send chan struct{}) {
	<-time.After(jitter())
	send <- struct{}{}
}
