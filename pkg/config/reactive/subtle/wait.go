// Functions in this package are useful in a few specific cases, but require
// careful consideration to ensure they are used correctly.
package subtle

import (
	"context"

	"github.com/rancher/opni/pkg/config/reactive"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// WaitOne returns the current value of the reactive.Value, or blocks until
// one becomes available, then returns it.
//
// WARNING: This function is intended for use in very few places, such as
// during startup when "bootstrapping" configuration that is needed to
// initialize the reactive controller itself. It should not be used in
// general purpose code, as it could introduce bugs that would be difficult
// to track down.
func WaitOne(ctx context.Context, val reactive.Value) protoreflect.Value {
	ctx, ca := context.WithCancel(ctx)
	defer ca()
	return <-val.Watch(ctx)
}

// WaitOneMessage is like WaitOne, but first wraps the given value in a
// reactive.Message.
//
// WARNING: This function is intended for use in very few places, such as
// during startup when "bootstrapping" configuration that is needed to
// initialize the reactive controller itself. It should not be used in
// general purpose code, as it could introduce bugs that would be difficult
// to track down.
func WaitOneMessage[T proto.Message](ctx context.Context, val reactive.Value) T {
	ctx, ca := context.WithCancel(ctx)
	defer ca()
	return <-reactive.Message[T](val).Watch(ctx)
}
