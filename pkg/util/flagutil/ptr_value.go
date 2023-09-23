package flagutil

import (
	"reflect"
	"strconv"

	"github.com/spf13/pflag"
	"golang.org/x/exp/constraints"
)

func Ptr[T any](t T) *T {
	return &t
}

type boolPtrFlag interface {
	pflag.Value
	IsBoolFlag() bool
}

type boolPtrValue struct {
	p **bool
}

func BoolPtrValue(val *bool, p **bool) pflag.Value {
	*p = val
	return &boolPtrValue{p}
}

func (b *boolPtrValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	*b.p = &v
	return err
}

func (b *boolPtrValue) Type() string {
	return "bool"
}

func (b *boolPtrValue) String() string {
	if *b.p == nil {
		return "nil"
	}
	return strconv.FormatBool(**b.p)
}

func (b *boolPtrValue) IsBoolFlag() bool { return true }

type intPtrValue[T constraints.Signed] struct {
	p **T
}

func IntPtrValue[T constraints.Signed](val *T, p **T) pflag.Value {
	*p = val
	return &intPtrValue[T]{p}
}

func (i *intPtrValue[T]) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		return err
	}
	tv := T(v)
	*i.p = &tv
	return nil
}

func (i *intPtrValue[T]) Type() string {
	return reflect.TypeOf(T(0)).Name()
}

func (i *intPtrValue[T]) String() string {
	if *i.p == nil {
		return "nil"
	}
	return strconv.FormatInt(int64(**i.p), 10)
}

type uintPtrValue[T constraints.Unsigned] struct {
	p **T
}

func UintPtrValue[T constraints.Unsigned](val *T, p **T) pflag.Value {
	*p = val
	return &uintPtrValue[T]{p}
}

func (i *uintPtrValue[T]) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		return err
	}
	tv := T(v)
	*i.p = &tv
	return nil
}

func (i *uintPtrValue[T]) Type() string {
	return reflect.TypeOf(T(0)).Name()
}

func (i *uintPtrValue[T]) String() string {
	if *i.p == nil {
		return "nil"
	}
	return strconv.FormatUint(uint64(**i.p), 10)
}

type floatPtrValue[T constraints.Float] struct {
	p **T
}

func FloatPtrValue[T constraints.Float](val *T, p **T) pflag.Value {
	*p = val
	return &floatPtrValue[T]{p}
}

func (i *floatPtrValue[T]) Set(s string) error {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	tv := T(v)
	*i.p = &tv
	return nil
}

func (i *floatPtrValue[T]) Type() string {
	return reflect.TypeOf(T(0)).Name()
}

func (i *floatPtrValue[T]) String() string {
	if *i.p == nil {
		return "nil"
	}
	return strconv.FormatFloat(float64(**i.p), 'g', -1, 64)
}

type stringPtrValue struct {
	p **string
}

func StringPtrValue(val *string, p **string) pflag.Value {
	*p = val
	return &stringPtrValue{p}
}

func (s *stringPtrValue) Set(v string) error {
	*s.p = &v
	return nil
}

func (s *stringPtrValue) Type() string {
	return "string"
}

func (s *stringPtrValue) String() string {
	if *s.p == nil {
		return "nil"
	}
	return **s.p
}
