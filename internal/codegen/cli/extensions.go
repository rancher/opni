package cli

import (
	"fmt"
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
		defer func() {
			if r := recover(); r != nil {
				panic(fmt.Sprintf("in %s: %s: failed to get extension %s from descriptor %T: %v", desc.ParentFile().Path(), desc.FullName(), ext.Name, desc, r))
			}
		}()
		// NB: proto.GetExtension does not work here. The line below does the same
		// thing except it uses Value.Interface instead of InterfaceOf.
		out, ok = desc.Options().ProtoReflect().Get(ext.TypeDescriptor()).Message().Interface().(T)
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
	if err := f.Default.UnmarshalTo(dm.Interface()); err != nil {
		panic(err)
	}
	orderedRange(dm, fn)
}

func orderedRange(dm protoreflect.Message, fn func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool) {
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
