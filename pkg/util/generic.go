package util

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"

	"github.com/iancoleman/strcase"
	"github.com/mitchellh/mapstructure"
	"github.com/rancher/opni/pkg/logger"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var (
	stackLg = logger.New(logger.WithZapOptions(zap.AddStacktrace(zap.InfoLevel)))
	errType = reflect.TypeOf((*error)(nil)).Elem()
)

func Must[T any](t T, err ...error) T {
	if len(err) > 0 {
		if err[0] != nil {
			stackLg.Panic(err)
		}
	}
	if typ := reflect.TypeOf(t); typ != nil && typ.Implements(errType) {
		stackLg.Panic(err)
	}
	return t
}

func DecodeStruct[T any](input interface{}) (*T, error) {
	output := new(T)
	config := &mapstructure.DecoderConfig{
		Metadata: nil,
		Result:   output,
		TagName:  "json",
		Squash:   true,
		MatchName: func(mapKey, fieldName string) bool {
			return strings.EqualFold(mapKey, fieldName) ||
				strings.EqualFold(strcase.ToSnake(mapKey), fieldName) ||
				strings.EqualFold(strcase.ToLowerCamel(mapKey), fieldName)
		},
	}

	// NewDecoder cannot fail - the only error condition is if
	// config.Result is not a pointer
	decoder := Must(mapstructure.NewDecoder(config))
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	return output, nil
}

func DeepCopyInto[T any](out, in *T) {
	Must(json.Unmarshal(Must(json.Marshal(in)), out))
}

func DeepCopy[T any](in *T) *T {
	out := new(T)
	DeepCopyInto(out, in)
	return out
}

func Pointer[T any](t T) *T {
	return &t
}

func ProtoClone[T proto.Message](msg T) T {
	return proto.Clone(msg).(T)
}

func IsInterfaceNil(i interface{}) bool {
	return reflect.ValueOf(i).Kind() == reflect.Ptr && reflect.ValueOf(i).IsNil()
}

type LockMap[K comparable, L sync.Locker] interface {
	Get(key K) L
	Delete(key K)
}

type locker[T any] interface {
	*T
	sync.Locker
}

type lockMapImpl[K comparable, L locker[T], T any] struct {
	locks   map[K]L
	locksMu sync.Mutex
}

func (lm *lockMapImpl[K, L, T]) Get(key K) L {
	lm.locksMu.Lock()
	defer lm.locksMu.Unlock()
	if l, ok := lm.locks[key]; ok {
		return l
	}
	l := new(T)
	lm.locks[key] = l
	return l
}

func (lm *lockMapImpl[K, L, T]) Delete(key K) {
	lm.locksMu.Lock()
	defer lm.locksMu.Unlock()
	delete(lm.locks, key)
}

func NewLockMap[K comparable, L locker[T], T any]() LockMap[K, L] {
	return &lockMapImpl[K, L, T]{
		locks: make(map[K]L),
	}
}

func BindContext[A any](f func(context.Context) A, ctx context.Context) func() A {
	return func() A {
		return f(ctx)
	}
}

func BindContext2[A, B any](f func(context.Context) (A, B), ctx context.Context) func() (A, B) {
	return func() (A, B) {
		return f(ctx)
	}
}
