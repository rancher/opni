package flagutil

import (
	"errors"

	"github.com/spf13/pflag"
	"golang.org/x/exp/constraints"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type protoenum[E any] interface {
	constraints.Integer
	Descriptor() protoreflect.EnumDescriptor
	Type() protoreflect.EnumType
	Number() protoreflect.EnumNumber
}

type enumValue[E protoenum[E]] struct {
	v *E
}

func EnumValue[E protoenum[E]](val E, p *E) pflag.Value {
	*p = val
	return &enumValue[E]{v: p}
}

func (e *enumValue[E]) Set(s string) error {
	vd := enumDescriptor[E]().Values().ByName(protoreflect.Name(s))
	if vd == nil {
		return errors.New("unknown enum value")
	}
	*e.v = E(vd.Number())
	return nil
}

func (e *enumValue[E]) String() string {
	return string(enumDescriptor[E]().Values().ByNumber(protoreflect.EnumNumber(*e.v)).Name())
}

func (e *enumValue[E]) Type() string {
	return string(enumDescriptor[E]().Name())
}

type enumPtrValue[E protoenum[E]] struct {
	v **E
}

func EnumPtrValue[E protoenum[E]](val *E, p **E) pflag.Value {
	*p = val
	return &enumPtrValue[E]{v: p}
}

func (e *enumPtrValue[E]) Set(s string) error {
	vd := enumDescriptor[E]().Values().ByName(protoreflect.Name(s))
	if vd == nil {
		return errors.New("unknown enum value")
	}
	ev := E(vd.Number())
	*e.v = &ev
	return nil
}

func (e *enumPtrValue[E]) String() string {
	if *e.v == nil {
		return "nil"
	}
	return string(enumDescriptor[E]().Values().ByNumber(protoreflect.EnumNumber(**e.v)).Name())
}

func (e *enumPtrValue[E]) Type() string {
	return string(enumDescriptor[E]().Name())
}

func enumDescriptor[E protoenum[E]]() protoreflect.EnumDescriptor {
	var e E
	return e.Descriptor()
}
