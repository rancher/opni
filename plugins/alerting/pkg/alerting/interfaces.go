package alerting

import (
	"context"
)

type WatcherHooks[T any] interface {
	RegisterEvent(eventType func(T) bool, hooks ...func(context.Context, T) error)
	HandleEvent(T)
}
