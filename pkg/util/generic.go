package util

import (
	"reflect"

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
