package etcd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

var (
	LockValidity   = 60 * time.Second
	LockRetryDelay = 200 * time.Millisecond
)

type EtcdLock struct {
	lg *slog.Logger

	prefix string
	key    string

	options *lock.LockOptions

	scheduler *lock.LockScheduler

	client *clientv3.Client
	mutex  *etcdMutex
}

func NewEtcdLock(
	lg *slog.Logger,
	client *clientv3.Client,
	prefix, key string,
	options *lock.LockOptions,
) *EtcdLock {
	return &EtcdLock{
		lg:        lg,
		client:    client,
		prefix:    prefix,
		key:       key,
		options:   options,
		scheduler: lock.NewLockScheduler(),
	}
}

func (e *EtcdLock) newSession(ctx context.Context) (*concurrency.Session, error) {
	e.lg.Debug("attempting to create new etcd session...")
	session, err := concurrency.NewSession(e.client)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd session: %w", err)
	}
	return session, nil
}

var _ storage.Lock = (*EtcdLock)(nil)

func (e *EtcdLock) acquire(ctx context.Context) (chan struct{}, error) {
	session, err := e.newSession(ctx)
	if err != nil {
		return nil, err
	}
	mutex := NewEtcdMutex(
		e.lg,
		e.prefix,
		e.key,
		session,
	)
	var curErr error
	done, err := mutex.lock(ctx)
	curErr = err
	if err == nil {
		e.mutex = &mutex
		return done, nil
	}
	return nil, curErr
}

func (e *EtcdLock) tryAcquire(ctx context.Context) (chan struct{}, error) {
	session, err := e.newSession(ctx)
	if err != nil {
		return nil, err
	}
	mutex := NewEtcdMutex(
		e.lg,
		e.prefix,
		e.key,
		session,
	)
	done, err := mutex.tryLock(ctx)
	var curErr = err
	if err == nil {
		e.mutex = &mutex
		return done, nil
	}
	return nil, curErr
}

func (e *EtcdLock) Lock(ctx context.Context) (chan struct{}, error) {
	e.lg.Debug("trying to acquire blocking lock")

	var closureDone chan struct{}
	if err := e.scheduler.Schedule(func() error {
		done, err := e.acquire(ctx)
		if err != nil {
			return err
		}
		closureDone = done
		return nil
	}); err != nil {
		return nil, err
	}
	e.lg.Debug("lock acquired", "chan", closureDone)
	return closureDone, nil
}

func (e *EtcdLock) TryLock(ctx context.Context) (acquired bool, done chan struct{}, err error) {
	e.lg.Debug("trying to acquire non-blocking lock")
	var closureDone chan struct{}
	if err := e.scheduler.Schedule(func() error {
		done, err := e.tryAcquire(ctx)
		if err != nil {
			return err
		}
		closureDone = done
		return nil
	}); err != nil {
		if errors.Is(err, concurrency.ErrLocked) {
			return false, nil, nil
		}
		return false, nil, err
	}

	e.lg.Debug("lock acquired", "chan", closureDone)
	return true, closureDone, nil
}

func (e *EtcdLock) Unlock() error {
	e.lg.Debug("starting unlock")

	if err := e.scheduler.Done(func() error {
		e.lg.Debug("inside scheduler done")
		if e.mutex == nil {
			panic("never acquired")
		}
		mutex := *e.mutex
		go func() {
			if err := mutex.unlock(); err != nil {
				e.lg.Error(err.Error())
			}
		}()
		e.mutex = nil
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (e *EtcdLock) Key() string {
	return e.key
}
