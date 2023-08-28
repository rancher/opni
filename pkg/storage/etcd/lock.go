package etcd

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

type EtcdLock struct {
	client  *clientv3.Client
	mutex   *concurrency.Mutex
	session *concurrency.Session

	Acquired uint32
	key      string

	startLock   lock.LockPrimitive
	startUnlock lock.LockPrimitive
	options     *lock.LockOptions
}

func NewEtcdLock(c *clientv3.Client, key string, options *lock.LockOptions) *EtcdLock {
	lockValiditySeconds := int(options.LockValidity.Seconds())
	if lockValiditySeconds == 0 {
		lockValiditySeconds = 1
	}
	s, err := concurrency.NewSession(c, concurrency.WithTTL(lockValiditySeconds))
	if err != nil {
		panic(err)
	}

	m := concurrency.NewMutex(s, key)
	return &EtcdLock{
		client:  c,
		session: s,
		mutex:   m,
		options: options,
		key:     key,
	}
}

var _ storage.Lock = (*EtcdLock)(nil)

func (e *EtcdLock) Lock() error {
	return e.startLock.Do(func() error {
		ctxca, ca := context.WithCancel(e.client.Ctx())
		signalAcquired := make(chan struct{})
		defer close(signalAcquired)
		if !e.options.Keepalive {
			defer e.session.Orphan()
		}
		var lockErr error
		var mu sync.Mutex
		go func() {
			select {
			case <-e.options.Ctx.Done():
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
		atomic.StoreUint32(&e.Acquired, 1)
		return nil
	})
}

func (e *EtcdLock) Unlock() error {
	return e.startUnlock.Do(func() error {
		if atomic.LoadUint32(&e.Acquired) == 0 {
			return lock.ErrLockNotAcquired
		}
		return e.mutex.Unlock(e.client.Ctx())
	})
}
