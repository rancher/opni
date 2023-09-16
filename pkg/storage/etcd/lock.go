package etcd

import (
	"context"
	"errors"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/etcd/concurrencyx"
	"github.com/rancher/opni/pkg/storage/lock"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

type EtcdLockManager struct {
	client  *clientv3.Client
	session *concurrency.Session
	prefix  string
}

// Locker implements storage.LockManager.
func (lm *EtcdLockManager) Locker(key string, opts ...lock.LockOption) storage.Lock {
	options := lock.DefaultLockOptions(lm.client.Ctx())
	options.Apply(opts...)
	m := concurrencyx.NewMutex(lm.session, path.Join(lm.prefix, key), options.InitialValue)
	return &EtcdLock{
		client:  lm.client,
		mutex:   m,
		options: options,
		prefix:  lm.prefix + "/",
	}
}

type EtcdLock struct {
	client *clientv3.Client
	mutex  *concurrencyx.Mutex

	acquired uint32

	prefix      string
	startLock   lock.LockPrimitive
	startUnlock lock.LockPrimitive
	options     *lock.LockOptions
}

var _ storage.Lock = (*EtcdLock)(nil)

func (e *EtcdLock) Lock() error {
	ctx := e.client.Ctx()
	if e.options.AcquireContext != nil {
		ctx = e.options.AcquireContext
	}
	return e.mutex.Lock(ctx)
	return e.startLock.Do(func() error {
		ctxca, ca := context.WithCancelCause(e.client.Ctx())
		signalAcquired := make(chan struct{})
		defer close(signalAcquired)
		var lockErr error
		var mu sync.Mutex
		go func() {
			select {
			case <-e.options.AcquireContext.Done():
				mu.Lock()
				lockErr = errors.Join(lockErr, lock.ErrAcquireLockCancelled)
				mu.Unlock()
				ca(lock.ErrAcquireLockCancelled)
			case <-time.After(e.options.AcquireTimeout):
				mu.Lock()
				lockErr = errors.Join(lockErr, lock.ErrAcquireLockTimeout)
				mu.Unlock()
				ca(lock.ErrAcquireLockTimeout)
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
	return e.mutex.Unlock(e.client.Ctx())

	return e.startUnlock.Do(func() error {
		if !atomic.CompareAndSwapUint32(&e.acquired, 1, 0) {
			return lock.ErrLockNotAcquired
		}
		return e.mutex.Unlock(e.client.Ctx())
	})
}

func (e *EtcdLock) Key() string {
	return strings.TrimPrefix(e.mutex.Key(), e.prefix)
}
