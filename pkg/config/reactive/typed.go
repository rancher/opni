package reactive

import (
	"context"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func Message[T proto.Message](rv Value) Reactive[T] {
	return &typedReactive[messageEncoder[T], T]{
		base: rv,
	}
}

func Scalar[T scalar](rv Value) Reactive[T] {
	return &typedReactive[scalarEncoder[T], T]{
		base: rv,
	}
}

type messageEncoder[T proto.Message] struct{}

func (messageEncoder[T]) FromValue(v protoreflect.Value) T {
	return v.Message().Interface().(T)
}

type scalarEncoder[T scalar] struct{}

func (scalarEncoder[T]) FromValue(v protoreflect.Value) T {
	return v.Interface().(T)
}

type typedReactive[E Encoder[T], T any] struct {
	base    Value
	encoder E
}

func (m *typedReactive[E, T]) Value() T {
	return m.encoder.FromValue(m.base.Value())
}

func (m *typedReactive[E, T]) Watch(ctx context.Context) <-chan T {
	wc := m.base.Watch(ctx)
	ch := make(chan T, 1)

	select {
	case v := <-wc:
		ch <- m.encoder.FromValue(v)
	default:
	}

	go func() {
		defer close(ch)
		for v := range wc {
			vt := m.encoder.FromValue(v)
			select {
			case ch <- vt:
			default:
				// replicate the behavior of reactiveMessage.update since we are
				// wrapping the original channel
				select {
				case <-ch:
				default:
				}
				ch <- vt
			}
		}
	}()
	return ch
}

func (m *typedReactive[E, T]) WatchFunc(ctx context.Context, onChanged func(T)) {
	m.base.WatchFunc(ctx, func(v protoreflect.Value) {
		onChanged(m.encoder.FromValue(v))
	})
}

func (m *typedReactive[E, T]) watchFuncWithRev(ctx context.Context, onChanged func(int64, T)) {
	m.base.watchFuncWithRev(ctx, func(rev int64, v protoreflect.Value) {
		onChanged(rev, m.encoder.FromValue(v))
	})
}

func (m *typedReactive[E, T]) wait() {
	m.base.wait()
}
