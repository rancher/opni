package inmemory

// https://man7.org/linux/man-pages/man2/flock.2.html
// https://man7.org/linux/man-pages/man2/fcntl.2.html
// https://gist.github.com/kawasin73/774ee48cb9a0c6fc7eb6c66d4392f9aa

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	golock "github.com/viney-shih/go-lock"
	"go.uber.org/zap"
)

type InMemoryLock struct {
	path string

	fh *os.File

	options *lock.LockOptions

	acquired        uint32
	unlocked        uint32
	startLock       lock.LockPrimitive
	startUnlock     lock.LockPrimitive
	expireTimestamp time.Time
	lg              *zap.SugaredLogger

	acquireMutex *golock.ChanMutex
}

func (i *InMemoryLock) Lock() error {
	return i.startLock.Do(func() error {
		endTime := time.Now().Add(i.options.LockValidity)
		ctx := i.options.AcquireContext
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGIO)
		go func() {
			defer close(c)
			for sig := range c {
				if sig != nil && sig == syscall.SIGIO { // another process is requesting the lease
					if !i.options.Keepalive && time.Now().After(i.expireTimestamp) {
						if err := i.Unlock(); err != nil {
							panic(err) // TODO : decide what to do
						}
					}
				}
			}
		}()
		ctx, ca := context.WithTimeout(ctx, i.options.AcquireTimeout)
		defer ca()
		ticker := time.NewTicker(i.options.RetryDelay)
		defer ticker.Stop()
		if locked := i.acquireMutex.TryLockWithContext(ctx); !locked {
			if ctx.Err() != nil {
				if errors.Is(ctx.Err(), context.Canceled) {
					return lock.ErrAcquireLockCancelled
				}
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return lock.ErrAcquireLockTimeout
				}
				return ctx.Err()
			}
		}
		if !i.options.Keepalive {
			go func() {
				<-time.After(time.Until(endTime))
				if atomic.CompareAndSwapUint32(&i.unlocked, 0, 1) {
					i.acquireMutex.Unlock()
					return
				}

			}()
		}
		for {
		RETRY:
			select {
			case <-ctx.Done():
				i.acquireMutex.Unlock()
				return ctx.Err()
			case <-ticker.C:

				flags := os.O_CREATE | os.O_RDWR

				fh, err := os.OpenFile(i.path, flags, os.FileMode(0600))
				if err != nil {
					goto RETRY
				}
				i.fh = fh

				flock := syscall.Flock_t{
					Start:  0,
					Len:    0,
					Type:   syscall.F_WRLCK,
					Whence: io.SeekStart,
				}

				// FLOCK is not appropriate because FLOCK on darwin does not return EINTR in blocking mode.
				if err := syscall.FcntlFlock(i.fh.Fd(), syscall.F_SETLKW, &flock); err != nil {
					i.lg.Error(err)
					// Retry on :
					// EBADF  fd is not an open file descriptor.

					// EINTR  While waiting to acquire a lock, the call was interrupted
					// by delivery of a signal caught by a handler; see
					// signal(7).
					goto RETRY
				}
				atomic.StoreUint32(&i.acquired, 1)
				return nil
			}
		}
	})
}

func (i *InMemoryLock) Unlock() error {
	return i.startUnlock.Do(func() error {
		if atomic.LoadUint32(&i.acquired) == 0 {
			return lock.ErrLockNotAcquired
		}
		defer func() {
			if atomic.CompareAndSwapUint32(&i.unlocked, 0, 1) {
				i.acquireMutex.Unlock()
			}
		}()

		flock := syscall.Flock_t{
			Start:  0,
			Len:    0,
			Type:   syscall.F_UNLCK,
			Whence: io.SeekStart,
		}
		if err := syscall.FcntlFlock(i.fh.Fd(), syscall.F_SETLKW, &flock); err != nil {
			return err
		}

		return i.fh.Close()
	})
}

func NewLock(ctx context.Context, path string, lg *zap.SugaredLogger, acquireMutex *golock.ChanMutex, opts ...lock.LockOption) storage.Lock {
	defaultOpts := lock.DefaultLockOptions(ctx)
	defaultOpts.Apply(opts...)
	return &InMemoryLock{
		path:         path,
		options:      defaultOpts,
		lg:           lg,
		acquireMutex: acquireMutex,
	}
}
