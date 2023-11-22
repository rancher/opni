package jetstream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/samber/lo"
)

func newLease(key string) *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:         key,
		Retention:    nats.InterestPolicy,
		Subjects:     []string{fmt.Sprintf("%s.lease.*", key)},
		MaxConsumers: 1,
	}
}

var (
	LockValidity   = 60 * time.Second
	LockRetryDelay = 100 * time.Millisecond
)

// encapsulates stateful information and tasks requried for holding a lock
type jetstreamMutex struct {
	lg *slog.Logger

	key  string
	uuid string

	js   nats.JetStreamContext
	msgQ chan *nats.Msg

	sub          *nats.Subscription
	internalDone chan struct{}
	retDone      chan struct{}
}

func newJetstreamMutex(
	lg *slog.Logger,
	js nats.JetStreamContext,
	key string,
) jetstreamMutex {
	uuid := uuid.New().String()
	return jetstreamMutex{
		js:           js,
		lg:           lg.With("uuid", uuid),
		key:          key,
		uuid:         uuid,
		msgQ:         make(chan *nats.Msg, 16),
		internalDone: make(chan struct{}),
		retDone:      make(chan struct{}),
	}
}

func (j *jetstreamMutex) tryLock() (chan struct{}, error) {
	var err error
	if _, err := j.js.AddStream(newLease(j.key)); err != nil {
		return nil, err
	}
	cfg := &nats.ConsumerConfig{
		Durable:           j.uuid,
		AckPolicy:         nats.AckExplicitPolicy,
		InactiveThreshold: LockValidity,
		DeliverSubject:    j.uuid,
		Heartbeat:         max(LockRetryDelay, 100*time.Millisecond),
	}
	if _, err := j.js.AddConsumer(j.key, cfg); err != nil {
		j.lg.Warn(err.Error())
		return nil, err
	}
	j.sub, err = j.js.ChanSubscribe(j.uuid, j.msgQ, nats.Bind(j.key, j.uuid))
	if err != nil {
		j.lg.Warn(err.Error())
		return nil, err
	}
	return lo.Async(j.keepaliveC), nil
}

func (j *jetstreamMutex) keepaliveC() struct{} {
	for {
		select {
		case <-j.internalDone:
			return struct{}{}
		case msg, ok := <-j.msgQ:
			if !ok {
				return struct{}{}
			}
			if err := msg.Ack(); err != nil {
				j.lg.Warn(fmt.Sprintf("failed to ack : %s", err.Error()))
			}
		}
	}
}

func (j *jetstreamMutex) teardown() {
	defer close(j.internalDone)
	select {
	case j.internalDone <- struct{}{}:
	default:
	}
}

func (j *jetstreamMutex) isReleased(err error) bool {
	return errors.Is(err, nats.ErrConsumerNotFound) || errors.Is(err, nats.ErrConsumerNotActive)
}

// !!Important : never treat nats closed connections as successful unlocks, this could lead to inconsistent states
func (j *jetstreamMutex) tryUnlock() error {
	if err := j.sub.Drain(); err != nil {
		j.lg.Error(fmt.Sprintf("failed to unsubscribe : %s", err.Error()))
		return err
	}
	consumerErr := j.js.DeleteConsumer(j.key, j.uuid)
	if consumerErr != nil && !j.isReleased(consumerErr) {
		j.lg.Error(fmt.Sprintf("failed to delete consumer : %s", consumerErr.Error()))
		return consumerErr
	}
	return nil
}

// best effort unlock until context is done, at which point we
// basically disconnect the connection keepalive semantic
// which delegates unlock the key to the KV server-side,
// giving the guarantee that unlock always actually unlocks when called
func (j *jetstreamMutex) unlock() error {
	defer j.teardown()
	ctx, ca := context.WithTimeout(context.Background(), 60*time.Second)
	defer ca()
	tTicker := time.NewTicker(LockRetryDelay)
	defer tTicker.Stop()

	// always try at least one unlock operation before ctx is done
	if err := j.tryUnlock(); err == nil {
		return nil
	}

	for {
		select {
		case <-tTicker.C:
			err := j.tryUnlock()
			if err == nil {
				return nil
			}
			j.lg.Warn(fmt.Sprintf("failed to unlock : %s, retrying...", err.Error()))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
