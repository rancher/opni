package alerting

import (
	"context"
	"github.com/nats-io/nats.go"
)

type JetStreamPublisher[T any] interface {
	GetIngressStream() string
	SetDurableOrderedPushConsumer(durableConsumer *nats.ConsumerConfig) error
	PublishLoop()
}

type ReplayJetStreamHooks[T any] interface {
	WatcherHooks[T]
	JetStreamPublisher[T]
}

type WatcherHooks[T any] interface {
	RegisterEvent(eventType func(T) bool, hooks ...func(context.Context, T) error)
	HandleEvent(T)
}

type InternalConditionWatcher interface {
	WatchEvents()
}
