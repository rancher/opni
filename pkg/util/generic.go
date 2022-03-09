package util

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"

	"github.com/iancoleman/strcase"
	"github.com/mitchellh/mapstructure"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"go.uber.org/zap"
)

var (
	stackLg = logger.New(logger.WithZapOptions(zap.AddStacktrace(zap.InfoLevel)))
	errType = reflect.TypeOf((*error)(nil)).Elem()
)

func Must[T any](t T, err ...error) T {
	if len(err) > 0 {
		if err[0] != nil {
			stackLg.Fatal(err)
		}
	}
	if typ := reflect.TypeOf(t); typ != nil && typ.Implements(errType) {
		stackLg.Fatal(err)
	}
	return t
}

func DecodeStruct[T any](input interface{}) (*T, error) {
	output := new(T)
	config := &mapstructure.DecoderConfig{
		Metadata: nil,
		Result:   output,
		TagName:  "json",
		MatchName: func(mapKey, fieldName string) bool {
			return strings.EqualFold(mapKey, fieldName) ||
				strings.EqualFold(strcase.ToSnake(mapKey), fieldName) ||
				strings.EqualFold(strcase.ToLowerCamel(mapKey), fieldName)
		},
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return nil, err
	}

	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	return output, nil
}

func DeepCopyInto[T any](out, in *T) {
	d, _ := json.Marshal(in)
	json.Unmarshal(d, out)
}

func DeepCopy[T any](in *T) *T {
	out := new(T)
	DeepCopyInto(out, in)
	return out
}

func Pointer[T any](t T) *T {
	return &t
}

type Future[T any] struct {
	once   sync.Once
	object T
	wait   chan struct{}
}

func NewFuture[T any]() *Future[T] {
	return &Future[T]{
		wait: make(chan struct{}),
	}
}

func (f *Future[T]) Set(object T) {
	f.once.Do(func() {
		f.object = object
		close(f.wait)
	})
}

func (f *Future[T]) Get() T {
	<-f.wait
	return f.object
}

func (f *Future[T]) GetContext(ctx context.Context) (_ T, err error) {
	select {
	case <-f.wait:
	case <-ctx.Done():
		err = ctx.Err()
		return
	}
	return f.object, nil
}
