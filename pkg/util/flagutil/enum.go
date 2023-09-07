package flagutil

import (
	"github.com/spf13/pflag"
	"github.com/thediveo/enumflag/v2"
	"golang.org/x/exp/constraints"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
)

type protoenum[E any] interface {
	constraints.Integer
	Descriptor() protoreflect.EnumDescriptor
	Type() protoreflect.EnumType
	Number() protoreflect.EnumNumber
}

func EnumValue[T protoenum[T]](p *T) pflag.Value {
	typeName := protoimpl.X.EnumTypeOf(*p).Descriptor().Name()
	mapping := make(map[T][]string)
	values := (*p).Type().Descriptor().Values()
	for i := 0; i < values.Len(); i++ {
		value := values.Get(i)
		mapping[T(value.Number())] = append(mapping[T(value.Number())], string(value.Name()))
	}
	return enumflag.New(p, string(typeName), mapping, enumflag.EnumCaseSensitive)
}
