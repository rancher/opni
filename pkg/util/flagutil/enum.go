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

func enumInfo[T protoenum[T]](p *T) (typename protoreflect.Name, mapping map[T][]string) {
	typename = protoimpl.X.EnumTypeOf(*p).Descriptor().Name()
	mapping = make(map[T][]string)
	values := (*p).Type().Descriptor().Values()
	for i := 0; i < values.Len(); i++ {
		value := values.Get(i)
		mapping[T(value.Number())] = append(mapping[T(value.Number())], string(value.Name()))
	}
	return
}
func EnumValue[T protoenum[T]](p *T) pflag.Value {
	typeName, mapping := enumInfo(p)
	return enumflag.New(p, string(typeName), mapping, enumflag.EnumCaseSensitive)
}

func EnumSliceValue[T protoenum[T]](p *[]T) pflag.Value {
	var t T
	typeName, mapping := enumInfo(&t)
	return enumflag.NewSlice[T](p, string(typeName), mapping, enumflag.EnumCaseSensitive)
}

func EnumPtrValue[T protoenum[T]](val *T, p **T) pflag.Value {
	typename, mapping := enumInfo(val)
	*p = val
	return enumflag.NewWithoutDefault(val, string(typename), mapping, enumflag.EnumCaseSensitive)
}
