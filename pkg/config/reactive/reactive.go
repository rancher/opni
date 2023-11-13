package reactive

import (
	"context"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type Value Reactive[protoreflect.Value]

type Reactive[T any] interface {
	reactiveInternal[T]
	Value() T
	Watch(ctx context.Context) <-chan T
	WatchFunc(ctx context.Context, onChanged func(value T))
}

type scalar interface {
	bool | int32 | int64 | uint32 | uint64 | float32 | float64 | string | []byte
}

type Encoder[T any] interface {
	FromValue(protoreflect.Value) T
}

type reactiveInternal[V any] interface {
	watchFuncWithRev(ctx context.Context, onChanged func(rev int64, value V))
	wait()
}
