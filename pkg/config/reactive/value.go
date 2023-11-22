package reactive

import (
	"context"
	"sync"

	"github.com/google/uuid"
	gsync "github.com/kralicky/gpkg/sync"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type reactiveValue struct {
	valueMu       sync.Mutex
	rev           int64
	value         protoreflect.Value
	watchChannels gsync.Map[string, chan protoreflect.Value]
	watchFuncs    gsync.Map[string, *func(int64, protoreflect.Value)]

	groupMu sync.Mutex
	group   <-chan struct{}
}

func (r *reactiveValue) Update(rev int64, v protoreflect.Value, group <-chan struct{}) {
	r.valueMu.Lock()
	r.rev = rev
	r.value = v
	r.valueMu.Unlock()

	r.groupMu.Lock()
	r.group = group
	r.groupMu.Unlock()

	r.watchChannels.Range(func(_ string, w chan protoreflect.Value) bool {
		select {
		case w <- v:
		default:
			// if the watch is not ready to receive, drop the previous value and
			// replace it with the current value
			select {
			case <-w:
				// current value discarded
			default:
				// the channel was read from just now
			}
			w <- v
		}
		return true
	})
	r.watchFuncs.Range(func(key string, value *func(int64, protoreflect.Value)) bool {
		(*value)(rev, v)
		return true
	})
}

func (r *reactiveValue) Value() protoreflect.Value {
	r.valueMu.Lock()
	defer r.valueMu.Unlock()
	return r.value
}

func (r *reactiveValue) Watch(ctx context.Context) <-chan protoreflect.Value {
	ch := make(chan protoreflect.Value, 1)

	r.valueMu.Lock()
	if r.rev != 0 {
		ch <- r.value
	}
	r.valueMu.Unlock()

	key := uuid.NewString()
	r.watchChannels.Store(key, ch)
	context.AfterFunc(ctx, func() {
		defer close(ch)
		r.watchChannels.Delete(key)
	})
	return ch
}

func (r *reactiveValue) WatchFunc(ctx context.Context, onChanged func(protoreflect.Value)) {
	r.watchFuncWithRev(ctx, func(_ int64, value protoreflect.Value) {
		onChanged(value)
	})
}

func (r *reactiveValue) watchFuncWithRev(ctx context.Context, onChanged func(int64, protoreflect.Value)) {
	r.valueMu.Lock()
	if r.rev != 0 {
		onChanged(r.rev, r.value)
	}
	r.valueMu.Unlock()

	key := uuid.NewString()
	r.watchFuncs.Store(key, &onChanged)
	context.AfterFunc(ctx, func() {
		r.watchFuncs.Delete(key)
	})
}

func (r *reactiveValue) wait() {
	r.groupMu.Lock()
	group := r.group
	r.groupMu.Unlock()

	if group == nil {
		return
	}
	<-group
}
