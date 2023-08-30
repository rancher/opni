package jetstream

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/util/future"
	"go.uber.org/zap"
)

var (
	GlobalId uint64
)

type JetstreamLock struct {
	lg      *zap.SugaredLogger
	connCtx context.Context

	key     string
	kv      nats.KeyValue
	options *lock.LockOptions

	startLock   lock.LockPrimitive
	startUnlock lock.LockPrimitive

	initialFencingToken future.Future[uint64]
	fencingToken        uint64

	acquiredLock uint32
	unlocked     uint32
}

func (j *JetstreamLock) Lock() error {
	return j.startLock.Do(func() error {
		acquireCtx, ca := context.WithTimeout(j.options.AcquireContext, j.options.AcquireTimeout)
		defer ca()
		t := time.NewTicker(j.options.RetryDelay)
		defer t.Stop()
		for {
		RETRY:
			select {
			case <-t.C:
				if err := j.tryLock(acquireCtx); err != nil {
					goto RETRY
				}
				atomic.StoreUint32(&j.acquiredLock, 1)
				go j.keepalive()
				return nil
			case <-acquireCtx.Done():
				if errors.Is(acquireCtx.Err(), context.DeadlineExceeded) {
					return errors.Join(lock.ErrAcquireLockTimeout, acquireCtx.Err())
				}
				if errors.Is(acquireCtx.Err(), context.Canceled) {
					return errors.Join(lock.ErrAcquireLockCancelled, acquireCtx.Err())
				}
				return acquireCtx.Err()
			}
		}
	})
}

func (j *JetstreamLock) tryLock(ctx context.Context) error {
	j.lg.Debug("trylock")
	revision, err := j.kv.Create(j.key, []byte("a"))
	if errors.Is(err, nats.ErrKeyExists) {
		j.lg.Debug("lock conflict, waiting for key update")
		watcher, err := j.kv.Watch(j.key, nats.MetaOnly(), nats.Context(ctx))
		if err != nil {
			return err
		}
		defer watcher.Stop()
		keyUpdates := watcher.Updates()
		for msg := range keyUpdates {
			if msg == nil {
				continue
			}
			j.lg.Debug("got key update", "operation", msg.Operation().String())
			switch msg.Operation() {
			case nats.KeyValueDelete, nats.KeyValuePurge:
				j.lg.Debug("key deleted, retrying lock")
				revision, err := j.kv.Create(j.key, []byte("a"))
				if err != nil {
					j.lg.Debug("failed to retry lock", "error", err)
					return err
				}
				j.lg.Debug("successfully retried lock")
				j.initialFencingToken.Set(revision)
				return nil
			}
		}
		j.lg.Debug("watcher stopped")
		return ctx.Err()
	} else if err != nil {
		return err
	}
	j.lg.Debug("successfully acquired lock")
	j.initialFencingToken.Set(revision)
	return nil
}

// blocks if keepalive is true
func (j *JetstreamLock) keepalive() {
	j.lg.Debug("keepalive started")
	fencingToken := j.initialFencingToken.Get()
	if j.options.Keepalive {
		t := time.NewTicker(DefaultLeaserExpireDuration / 5)
		for {
			select {
			case <-t.C:
				j.lg.Debug("keepalive")
				revision, err := j.kv.Update(j.key, []byte("a"), fencingToken)
				if err != nil {
					panic(err)
				}
				atomic.CompareAndSwapUint64(&j.fencingToken, fencingToken, revision)
				fencingToken = revision

			case <-j.connCtx.Done():
				j.lg.Debug("keepalive stopped")
				j.tryUnlock()
			}
		}
	}
}

func (j *JetstreamLock) Unlock() error {
	return j.startUnlock.Do(func() error {
		if atomic.LoadUint32(&j.acquiredLock) == 0 {
			return lock.ErrLockNotAcquired
		}
		return j.tryUnlock()
	})
}

func (j *JetstreamLock) tryUnlock() error {
	if atomic.CompareAndSwapUint32(&j.unlocked, 0, 1) {
		j.lg.Debug("unlock")
		if err := j.kv.Delete(j.key); err != nil {
			j.lg.Error("failed to unlock", "error", err)
			return err
		}
	}
	return nil
}

func NewLock(connCtx context.Context, kv nats.KeyValue, key string, options *lock.LockOptions, lg *zap.SugaredLogger) *JetstreamLock {
	atomic.AddUint64(&GlobalId, 1)
	return &JetstreamLock{
		kv:                  kv,
		connCtx:             connCtx,
		key:                 key,
		options:             options,
		initialFencingToken: future.New[uint64](),
		lg:                  lg.With("id", GlobalId),
	}
}
