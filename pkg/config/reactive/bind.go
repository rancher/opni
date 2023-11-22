package reactive

import (
	"context"
	"sync"

	gsync "github.com/kralicky/gpkg/sync"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Bind groups multiple reactive.Value instances together, de-duplicating
// updates using the revision of the underlying config.
//
// The callback is invoked when one or more reactive.Values change,
// and is passed the current or updated value of each reactive value, in the
// order they were passed to Bind. Values that have never been set will be
// invalid (protoreflect.Value.IsValid() returns false).
//
// For partial updates, the values passed to the callback will be either the
// updated value or the current value, depending on whether the value was
// updated in the current revision.
//
// The callback is guaranteed to be invoked exactly once for a single change
// to the active config, even if multiple values in the group change at the
// same time. The values must all be created from the same controller,
// otherwise the behavior is undefined.
func Bind(ctx context.Context, callback func([]protoreflect.Value), reactiveValues ...Value) {
	b := &binder{
		reactiveValues: reactiveValues,
		callback:       callback,
	}
	for i, rv := range reactiveValues {
		i := i
		rv.watchFuncWithRev(ctx, func(rev int64, v protoreflect.Value) {
			b.onUpdate(i, rev, v)
		})
	}
}

type binder struct {
	callback       func([]protoreflect.Value)
	reactiveValues []Value
	queues         gsync.Map[int64, *queuedUpdate]
}

type queuedUpdate struct {
	lazyInit sync.Once
	values   []protoreflect.Value
	resolve  sync.Once
}

func (q *queuedUpdate) doLazyInit(size int) {
	q.lazyInit.Do(func() {
		q.values = make([]protoreflect.Value, size)
	})
}

func (b *binder) onUpdate(i int, rev int64, v protoreflect.Value) {
	q, _ := b.queues.LoadOrStore(rev, &queuedUpdate{})
	q.doLazyInit(len(b.reactiveValues))
	// this *must* happen synchronously, since the group channel is closed
	// once all callbacks have returned.
	q.values[i] = v

	go func() {
		b.reactiveValues[i].wait()
		q.resolve.Do(func() {
			b.queues.Delete(rev)
			b.doResolve(q)
		})
	}()
}

func (b *binder) doResolve(q *queuedUpdate) {
	for i, v := range q.values {
		if !v.IsValid() {
			q.values[i] = b.reactiveValues[i].Value()
		}
	}
	b.callback(q.values)
}
