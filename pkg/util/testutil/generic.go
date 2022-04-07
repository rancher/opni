package testutil

import (
	"os"
	"reflect"

	. "github.com/onsi/gomega"
)

var (
	errType = reflect.TypeOf((*error)(nil)).Elem()
)

func Must[T any](t T, err ...error) T {
	Expect(err).To(
		Or(
			HaveLen(0),
			And(
				HaveLen(1),
				ContainElement(BeNil()),
			),
		),
	)
	typ := reflect.TypeOf(t)
	Expect(typ).To(
		Or(
			BeNil(),
			WithTransform(
				func(t reflect.Type) bool {
					return t.Implements(errType)
				},
				BeFalse(),
			),
		),
	)
	return t
}

type ifCIExpr[T any] struct {
	value T
}

func IfCI[T any](t T) ifCIExpr[T] {
	return ifCIExpr[T]{
		value: t,
	}
}

func (e ifCIExpr[T]) Else(t T) T {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return e.value
	}
	return t
}
