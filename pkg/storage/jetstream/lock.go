package jetstream

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
)

func newLease(key string) *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:         key,
		Retention:    nats.InterestPolicy,
		Subjects:     []string{fmt.Sprintf("%s.lease.*", key)},
		MaxConsumers: 1,
	}
}

type Lock struct {
	key  string
	uuid string

	acquired uint32

	js   nats.JetStreamContext
	sub  *nats.Subscription
	msgQ chan *nats.Msg
	*lock.LockOptions

	startLock   lock.LockPrimitive
	startUnlock lock.LockPrimitive
}

var _ storage.Lock = (*Lock)(nil)

func NewLock(js nats.JetStreamContext, key string, options *lock.LockOptions) *Lock {
	return &Lock{
		key:         key,
		js:          js,
		uuid:        uuid.New().String(),
		msgQ:        make(chan *nats.Msg, 16),
		LockOptions: options,
	}
}

func (l *Lock) Lock() error {

	return l.startLock.Do(func() error {
		return l.lock()
	})
}

func (l *Lock) lock() error {
	timeout := time.After(l.AcquireTimeout)
	tTicker := time.NewTicker(l.RetryDelay)
	defer tTicker.Stop()

	var lockErr error
	for {
		select {
		case <-tTicker.C:
			err := l.tryLock()
			if err == nil {
				return nil
			}
			lockErr = err
		case <-l.AcquireContext.Done():
			return errors.Join(lock.ErrAcquireLockCancelled, lockErr)
		case <-timeout:
			return errors.Join(lockErr, lock.ErrAcquireLockTimeout)
		}
	}
}

func (l *Lock) tryLock() error {
	var err error
	if _, err := l.js.AddStream(newLease(l.key)); err != nil {
		return err
	}
	cfg := &nats.ConsumerConfig{
		Durable:           l.uuid,
		AckPolicy:         nats.AckExplicitPolicy,
		InactiveThreshold: l.LockValidity,
		DeliverSubject:    l.uuid,
	}
	if l.Keepalive {
		// Push-based details, defaults to 5 * time.Second
		cfg.Heartbeat = max(l.RetryDelay, 100*time.Millisecond)
	}

	if _, err := l.js.AddConsumer(l.key, cfg); err != nil {
		return err
	}
	l.sub, err = l.js.ChanSubscribe(l.uuid, l.msgQ, nats.Bind(l.key, l.uuid))
	if err != nil {
		return err
	}
	go l.keepalive()
	go l.expire()
	atomic.StoreUint32(&l.acquired, 1)
	return nil
}

func (l *Lock) expire() {
	if l.Keepalive {
		return
	}
	timeout := time.After(l.LockValidity)
	<-timeout
	l.unlock()
}

func (l *Lock) keepalive() {
	if !l.Keepalive {
		return
	}
	for msg := range l.msgQ {
		msg.Ack()
	}
}

func (l *Lock) Unlock() error {
	return l.startUnlock.Do(func() error {
		if atomic.LoadUint32(&l.acquired) == 0 {
			return lock.ErrLockNotAcquired
		}
		return l.unlock()
	})
}

func (l *Lock) unlock() error {
	timeout := time.After(l.AcquireTimeout)
	tTicker := time.NewTicker(l.RetryDelay)
	defer tTicker.Stop()

	var unlockErr error
	for {
		select {
		case <-tTicker.C:
			err := l.tryUnlock()
			if err == nil {
				return nil
			}
			unlockErr = err
		case <-l.AcquireContext.Done():
			return errors.Join(lock.ErrAcquireUnlockCancelled, unlockErr)
		case <-timeout:
			return errors.Join(lock.ErrAcquireUnlockTimeout, unlockErr)
		}
	}
}

func (l *Lock) tryUnlock() error {
	if err := l.sub.Unsubscribe(); err != nil {
		if errors.Is(err, nats.ErrBadSubscription) || errors.Is(err, nats.ErrConsumerNotActive) { // lock already released
			return nil
		}
		return err
	}
	if err := l.js.DeleteConsumer(l.key, l.uuid); err != nil {
		if errors.Is(err, nats.ErrConsumerNotFound) || errors.Is(err, nats.ErrConsumerNotActive) { // lock already released
			return nil
		}
		return err
	}
	return nil
}
