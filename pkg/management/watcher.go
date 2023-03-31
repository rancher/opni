package management

import (
	"context"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
)

type ConditionWatcher interface {
	WatchEvents()
}

type internalWatcher struct {
	closures []func()
}

func NewConditionWatcher(cl ...func()) ConditionWatcher {
	return &internalWatcher{
		closures: cl,
	}
}

func (w *internalWatcher) WatchEvents() {
	for _, cl := range w.closures {
		cl := cl
		go func() {
			cl()
		}()
	}
}

type WatcherHooks[T any] interface {
	RegisterEvent(eventType func(T) bool, hooks ...func(context.Context, T) error)
	HandleEvent(T)
}

type ManagementWatcherHooks[T proto.Message] struct {
	parentCtx context.Context
	hooks     []lo.Tuple2[func(T) bool, []func(context.Context, T) error]
}

func NewManagementWatcherHooks[T proto.Message](ctx context.Context) *ManagementWatcherHooks[T] {
	return &ManagementWatcherHooks[T]{
		parentCtx: ctx,
		hooks:     make([]lo.Tuple2[func(T) bool, []func(context.Context, T) error], 0),
	}
}

func (h *ManagementWatcherHooks[T]) RegisterHook(filter func(T) bool, hooks ...func(context.Context, T) error) {
	h.hooks = append(h.hooks, lo.Tuple2[func(T) bool, []func(context.Context, T) error]{
		A: filter,
		B: hooks,
	})
}

func (h *ManagementWatcherHooks[T]) HandleEvent(event T) {
	for _, cs := range h.hooks {
		if cs.A(event) {
			for _, hook := range cs.B {
				hook(h.parentCtx, event)
			}
		}
	}
}
