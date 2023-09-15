package etcd

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/etcd/concurrencyx"
	"github.com/rancher/opni/pkg/storage/lock"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdLock struct {
	client *clientv3.Client
	mutex  *concurrencyx.Mutex

	acquired uint32

	startLock   lock.LockPrimitive
	startUnlock lock.LockPrimitive
	options     *lock.LockOptions
}

var _ storage.Lock = (*EtcdLock)(nil)

func (e *EtcdLock) Lock() error {
	return e.startLock.Do(func() error {
		ctxca, ca := context.WithCancel(e.options.AcquireContext)
		var lockErr error
		var mu sync.Mutex
		go func() {
			select {
			case <-e.options.AcquireContext.Done():
				mu.Lock()
				lockErr = errors.Join(lockErr, lock.ErrAcquireLockCancelled)
				mu.Unlock()
				ca()
			case <-time.After(e.options.AcquireTimeout):
				mu.Lock()
				lockErr = errors.Join(lockErr, lock.ErrAcquireLockTimeout)
				mu.Unlock()
				ca()
			}
		}()
		err := e.mutex.Lock(ctxca)
		mu.Lock()
		err = errors.Join(lockErr, err)
		mu.Unlock()
		if err != nil {
			e.mutex.Unlock(e.client.Ctx())
			return err
		}
		atomic.StoreUint32(&e.acquired, 1)
		return nil
	})
}

func (e *EtcdLock) Unlock() error {
	return e.startUnlock.Do(func() error {
		if atomic.LoadUint32(&e.acquired) == 0 {
			return lock.ErrLockNotAcquired
		}
		return e.mutex.Unlock(e.client.Ctx())
	})
}

func (e *EtcdLock) Key() string {
	return e.mutex.Key()
}
