package driverutil

import (
	reflect "reflect"
	"sync"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func GetRevisionFieldIndex[T Revisioner]() int {
	var revision corev1.Revision
	revisionFqn := revision.ProtoReflect().Descriptor().FullName()
	var t T
	fields := t.ProtoReflect().Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if field.Kind() == protoreflect.MessageKind {
			if field.Message().FullName() == revisionFqn {
				return i
			}
		}
	}
	panic("revision field not found")
}

var (
	indexCacheMu sync.Mutex
	indexCache   = map[reflect.Type]func() int{}
)

func UnsetRevision[T Revisioner](t T) {
	typ := reflect.TypeOf(t)
	indexCacheMu.Lock()
	if _, ok := indexCache[typ]; !ok {
		indexCache[typ] = sync.OnceValue(GetRevisionFieldIndex[T])
	}
	idx := indexCache[typ]()
	indexCacheMu.Unlock()
	if rev := t.GetRevision(); rev != nil {
		field := t.ProtoReflect().Descriptor().Fields().Get(idx)
		t.ProtoReflect().Clear(field)
	}
}

func SetRevision[T Revisioner](t T, value int64, maybeTimestamp ...time.Time) {
	typ := reflect.TypeOf(t)
	indexCacheMu.Lock()
	if _, ok := indexCache[typ]; !ok {
		indexCache[typ] = sync.OnceValue(GetRevisionFieldIndex[T])
	}
	idx := indexCache[typ]()
	indexCacheMu.Unlock()
	if rev := t.GetRevision(); rev == nil {
		field := t.ProtoReflect().Descriptor().Fields().Get(idx)
		updatedRev := &corev1.Revision{Revision: &value}
		if len(maybeTimestamp) > 0 && !maybeTimestamp[0].IsZero() {
			updatedRev.Timestamp = timestamppb.New(maybeTimestamp[0])
		}
		t.ProtoReflect().Set(field, protoreflect.ValueOfMessage(updatedRev.ProtoReflect()))
	} else {
		rev.Set(value)
	}
}

func CopyRevision[T Revisioner](dst, src T) {
	typ := reflect.TypeOf(dst)
	indexCacheMu.Lock()
	if _, ok := indexCache[typ]; !ok {
		indexCache[typ] = sync.OnceValue(GetRevisionFieldIndex[T])
	}
	idx := indexCache[typ]()
	indexCacheMu.Unlock()

	if srcRev := src.GetRevision(); srcRev != nil {
		field := dst.ProtoReflect().Descriptor().Fields().Get(idx)
		dst.ProtoReflect().Set(field, protoreflect.ValueOfMessage(srcRev.ProtoReflect()))
	} else {
		UnsetRevision(dst)
	}
}
