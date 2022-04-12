package testutil

import (
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
