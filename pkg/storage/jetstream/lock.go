package jetstream

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

type jetstreamLockMetadata struct {
	AccessType lock.AccessType `json:"accessType"`
	// UnixNano
	ExpireTime int64 `json:"expireTime"`
	// lock manager uuid's that hold this lock
	Holders map[string]struct{} `json:"holders"`
}

type JetstreamLock struct {
	lg       *zap.SugaredLogger
	kv       nats.KeyValue
	lockPool *lock.LockPool

	kvCtx  context.Context
	lockMu sync.Mutex

	key         string
	managerUuid string

	lockRequestTs future.Future[int64]
	fencingToken  future.Future[uint64]

	*lock.LockOptions

	startLock   lock.LockPrimitive
	startUnlock lock.LockPrimitive
}

func NewJetstreamLock(
	ctx context.Context,
	lg *zap.SugaredLogger,
	kv nats.KeyValue,
	key string,
	options *lock.LockOptions,
	lockPool *lock.LockPool,
	uuid string,
) *JetstreamLock {
	GlobalLockId++
	return &JetstreamLock{
		lg:            lg.With("id", GlobalLockId, "type", lock.AccessTypeToString(options.AccessType)),
		kvCtx:         ctx,
		kv:            kv,
		key:           key,
		lockRequestTs: future.New[int64](),
		fencingToken:  future.New[uint64](),
		LockOptions:   options,
		startLock:     lock.LockPrimitive{},
		startUnlock:   lock.LockPrimitive{},
		lockPool:      lockPool,
		managerUuid:   uuid,
	}
}

func (j *JetstreamLock) jitter() time.Duration {
	return time.Duration(j.MaxRetries) * time.Duration(j.RetryDelay) / 2
}

func (j *JetstreamLock) Lock() error {
	return j.startLock.Do(func() error {
		lg := j.lg.With("transaction", "lock")
		lg.Debug("REQ init")
		lg.Debugf("WITH ACQQUISITION TIMEOUT %s", j.AcquireTimeout.String())
		j.lockRequestTs.Set(time.Now().UnixNano())
		maxRetries := j.MaxRetries

		cancelAcquire := make(chan struct{})
		lockAcquired := make(chan error)
		lockErr := make(chan error)

		go func() {
			defer close(lockErr)
			defer close(cancelAcquire)
			select {
			case <-j.kvCtx.Done():
				lg.Debug("TIMEOUT kv cancel")
				lockErr <- errors.Join(lock.ErrAcquireLockCancelled, j.kvCtx.Err())
				cancelAcquire <- struct{}{}
			case <-j.Ctx.Done():
				lg.Debug("TIMEOUT client cancel")
				lockErr <- errors.Join(lock.ErrAcquireLockCancelled, j.Ctx.Err())
				cancelAcquire <- struct{}{}
			case <-time.After(j.AcquireTimeout):
				lg.Debug("TIMEOUT acquire timeout")
				lockErr <- lock.ErrAcquireLockTimeout
				cancelAcquire <- struct{}{}
			case err := <-lockAcquired:
				lockErr <- err
			}
		}()

		go func() {
			// defer watcher.Stop()
			defer close(lockAcquired)
			for retries := 0; retries < maxRetries; retries++ {
				select {
				case <-cancelAcquire:
				case <-time.After(j.RetryDelay + j.jitter()):
					// retry
					lg.Debug("REQ")
					err := j.lock(j.kvCtx)
					if err == nil {
						lockAcquired <- nil
						return
					}
					if errors.Is(err, lock.ErrAcquireLockConflict) {
						lg.Debug("ACK err conflict")
					} else {
						lg.Debugf("ACK err : %s", err.Error())
					}
				}
			}
			lockAcquired <- lock.ErrAcquireLockRetryExceeded
		}()

		err := <-lockErr
		return err
	})
}

func (j *JetstreamLock) forceRelease(
	token uint64,
) error {
	j.lg.Debugf("DEL : %s force", j.key)
	return j.kv.Purge(j.key, nats.LastRevision(token))
}

func (j *JetstreamLock) release() error {
	if holds := j.lockPool.Holds(j.key); holds { // some other client registered to this manager holds a lock
		return nil
	}

	lock, err := j.kv.Get(j.key)
	if err != nil && errors.Is(err, nats.ErrKeyNotFound) {
		return nil
	}

	var md jetstreamLockMetadata
	if err := json.Unmarshal(lock.Value(), &md); err != nil {
		return err
	}

	_, remotelyHolds := md.Holders[j.managerUuid]
	if remotelyHolds {
		delete(md.Holders, j.managerUuid)
	}
	if len(md.Holders) > 0 {
		mdData, err := json.Marshal(md)
		if err != nil {
			return err
		}
		_, err = j.kv.Update(j.key, mdData, lock.Revision())
		if err != nil {
			return err
		}
		return nil
	} else if len(md.Holders) == 0 {
		return j.forceRelease(lock.Revision())
	}
	return nil
}

func (j *JetstreamLock) Unlock() error {
	return j.startUnlock.Do(func() error {
		lg := j.lg.With("transaction", "unlock")
		lg.Debug("REQ")
		if !j.isAcquired() {
			return lock.ErrLockNotAcquired
		}
		maxRetries := j.MaxRetries
		cancelAcquire := make(chan struct{})
		unlockAcquired := make(chan error)
		unlockErr := make(chan error)
		j.releaseLock()

		go func() {
			defer close(unlockErr)
			defer close(cancelAcquire)
			select {
			case <-j.kvCtx.Done():
				lg.Debug("TIMEOUT kv cancel")
				unlockErr <- errors.Join(lock.ErrAcquireUnlockCancelled, j.kvCtx.Err())
				cancelAcquire <- struct{}{}
			case <-j.Ctx.Done():
				lg.Debug("TIMEOUT client cancel")
				unlockErr <- errors.Join(lock.ErrAcquireUnlockCancelled, j.Ctx.Err())
				cancelAcquire <- struct{}{}
			case <-time.After(time.Second):
				lg.Debug("TIMEOUT acquire timeout")
				unlockErr <- lock.ErrAcquireUnlockTimeout
				cancelAcquire <- struct{}{}
			case err := <-unlockAcquired:
				unlockErr <- err
			}
		}()

		go func() {
			for retries := 0; retries < maxRetries; retries++ {
				select {
				case <-cancelAcquire:
					return
				case <-time.After(j.RetryDelay + j.jitter()):
					err := j.release()
					if err == nil {
						lg.Debug("ACK")
						unlockAcquired <- nil
						return
					}
				}
			}
			lg.Debug("TIMEOUT retry")
			unlockErr <- lock.ErrAcquireUnlockTimeout
		}()

		err := <-unlockErr
		return err
	})
}

func (j *JetstreamLock) isAcquired() bool {
	return j.fencingToken.IsSet()
}

func (j *JetstreamLock) lock(ctx context.Context) error {
	j.lockMu.Lock()
	defer j.lockMu.Unlock()
	if j.isAcquired() {
		return nil
	}
	lockMd, err := j.getLockMd()
	found := err == nil
	notFound := errors.Is(err, nats.ErrKeyNotFound)
	if notFound {
		j.lg.Debug("CRT new")
		return j.tryAcquireNewLock(ctx)
	}
	if found && j.isCompat(lockMd) {
		j.lg.Debug("CRT existing")
		return j.tryAcquireLock(ctx, lockMd)
	}
	if found && j.isExpired(lockMd) {
		j.lg.Debug("RLS : expired")
		if err := j.forceRelease(lockMd.Revision()); err != nil {
			return err
		}
		return lock.ErrAcquireLockConflict
	}
	if found {
		j.lg.Debug("WAIT")
		return lock.ErrAcquireLockConflict
	}
	return err
}

func (j *JetstreamLock) isExpired(incomingLockMd nats.KeyValueEntry) bool {
	cur := time.Now().UnixNano()
	var md jetstreamLockMetadata
	if err := json.Unmarshal(incomingLockMd.Value(), &md); err != nil {
		panic(err)
	}
	return cur > md.ExpireTime
}

func (j *JetstreamLock) getLockMd() (nats.KeyValueEntry, error) {
	return j.kv.Get(j.key)
}

func (j *JetstreamLock) tryAcquireNewLock(ctx context.Context) error {
	md, err := j.currentLockMetadata(ctx)
	if err != nil {
		return err
	}
	mdData, err := json.Marshal(md)
	if err != nil {
		return err
	}
	fencingToken, err := j.kv.Create(j.key, mdData)
	if errors.Is(nats.ErrKeyExists, err) {
		return lock.ErrAcquireLockConflict
	}
	if err != nil {
		return err
	}
	j.acquireLock(fencingToken)
	return nil
}

// this is an atomic operation, should only ever be run once per lock on success
func (j *JetstreamLock) acquireLock(fencingToken uint64) {
	j.lg.Debugf("ACQUIRED LOCK with fencing token %d", fencingToken)
	j.fencingToken.Set(fencingToken)
	j.lockPool.AddLock(j.key, fencingToken)
}

// atomic; returns if this lock manager no longer holds any locks
func (j *JetstreamLock) releaseLock() bool {
	fencingToken := j.fencingToken.Get()
	j.lg.Debugf("RELEASED LOCK with fencing token %d", fencingToken)
	return j.lockPool.RemoveLock(j.key, fencingToken)
}

func (j *JetstreamLock) tryAcquireLock(ctx context.Context, lockMessage nats.KeyValueEntry) error {
	var theirMd jetstreamLockMetadata
	if err := json.Unmarshal(lockMessage.Value(), &theirMd); err != nil {
		return err
	}

	ourMd, err := j.currentLockMetadata(ctx)
	if err != nil {
		return err
	}
	ourMd.AccessType = lo.Ternary(ourMd.AccessType < theirMd.AccessType, theirMd.AccessType, ourMd.AccessType)
	ourMd.Holders = lo.Assign(ourMd.Holders, theirMd.Holders)

	mdData, err := json.Marshal(ourMd)
	if err != nil {
		return err
	}

	updatedFencingToken, err := j.kv.Update(j.key, mdData, lockMessage.Revision())
	if err != nil {
		return lock.ErrAcquireLockConflict
	}
	j.acquireLock(updatedFencingToken)
	return nil
}

func convertUnixNano(un int64) time.Time {
	return time.Unix(0, un)
}

func (j *JetstreamLock) currentLockMetadata(ctx context.Context) (*jetstreamLockMetadata, error) {
	ts, err := j.lockRequestTs.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	expire := convertUnixNano(ts).Add(j.LockValidity)
	return &jetstreamLockMetadata{
		AccessType: j.AccessType,
		ExpireTime: expire.UnixNano(),
		Holders: map[string]struct{}{
			j.managerUuid: {},
		},
	}, nil
}

func (j *JetstreamLock) isCompat(entry nats.KeyValueEntry) bool {
	var metadata jetstreamLockMetadata
	if err := json.Unmarshal(entry.Value(), &metadata); err != nil {
		panic(err)
	}
	j.lg.Debugf("checking if ours : %s is compatible with theirs : %s", lock.AccessTypeToString(j.AccessType), lock.AccessTypeToString(metadata.AccessType))
	return lock.Compat(j.AccessType, metadata.AccessType)
}
