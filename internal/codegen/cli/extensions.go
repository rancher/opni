package cli

import (
	"slices"

	"github.com/samber/lo"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
	"google.golang.org/protobuf/types/dynamicpb"
)

func getExtension[T proto.Message](desc protoreflect.Descriptor, ext *protoimpl.ExtensionInfo) (out T, ok bool) {
	if proto.HasExtension(desc.Options(), ext) {
		out, ok = proto.GetExtension(desc.Options(), ext).(T)
	}
	return
}

func getGeneratorOptions(file protoreflect.Descriptor) (*GeneratorOptions, bool) {
	return getExtension[*GeneratorOptions](file, E_Generator)
}

func getCommandGroupOptions(svc protoreflect.Descriptor) (*CommandGroupOptions, bool) {
	return getExtension[*CommandGroupOptions](svc, E_CommandGroup)
}

func getCommandOptions(mtd protoreflect.Descriptor) (*CommandOptions, bool) {
	return getExtension[*CommandOptions](mtd, E_Command)
}

func getFlagOptions(fld protoreflect.Descriptor) (*FlagOptions, bool) {
	return getExtension[*FlagOptions](fld, E_Flag)
}

func getFlagSetOptions(fld protoreflect.Descriptor) (*FlagSetOptions, bool) {
	return getExtension[*FlagSetOptions](fld, E_FlagSet)
}

func applyOptions(desc protoreflect.Descriptor, out proto.Message) {
	var opts proto.Message
	var ok bool
	switch out.(type) {
	case *GeneratorOptions:
		opts, ok = getGeneratorOptions(desc)
	case *CommandGroupOptions:
		opts, ok = getCommandGroupOptions(desc)
	case *CommandOptions:
		opts, ok = getCommandOptions(desc)
	case *FlagOptions:
		opts, ok = getFlagOptions(desc)
	case *FlagSetOptions:
		opts, ok = getFlagSetOptions(desc)
	}
	if ok {
		proto.Merge(out, opts)
	}
}

func (f *FlagSetOptions) ForEachDefault(fieldMessage *protogen.Message, fn func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool) {
	if f.Default == nil {
		return
	}
	dm := dynamicpb.NewMessage(fieldMessage.Desc)
	f.Default.UnmarshalTo(dm)
	orderedRange(dm, fn)
}

func orderedRange(dm *dynamicpb.Message, fn func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool) {
	ordered := []lo.Tuple2[protoreflect.FieldDescriptor, protoreflect.Value]{}
	dm.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		ordered = append(ordered, lo.T2(fd, v))
		return true
	})
	slices.SortFunc(ordered, func(a, b lo.Tuple2[protoreflect.FieldDescriptor, protoreflect.Value]) int {
		return int(a.A.Number() - b.A.Number())
	})
	for _, t := range ordered {
		if !fn(t.A, t.B) {
			return
		}
	}
}
