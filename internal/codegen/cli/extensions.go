package cli

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
	"google.golang.org/protobuf/types/dynamicpb"
)

func getExtension[T proto.Message](desc protoreflect.Descriptor, ext *protoimpl.ExtensionInfo) (T, bool) {
	var t T
	if proto.HasExtension(desc.Options(), ext) {
		defer func() {
			if r := recover(); r != nil {
				panic(fmt.Sprintf("in %s: %s: failed to get extension %s from descriptor %T: %v", desc.ParentFile().Path(), desc.FullName(), ext.Name, desc, r))
			}
		}()

		// NB: proto.GetExtension does not work here. The line below does the same
		// thing except it uses Value.Interface instead of InterfaceOf.
		// Additionally, we need to handle dynamic extensions since we're inside
		// a plugin.
		msg := desc.Options().ProtoReflect().Get(ext.TypeDescriptor()).Message().Interface()
		if t, ok := msg.(T); ok {
			return t, true
		}
		t = t.ProtoReflect().New().Interface().(T)
		if dt, ok := msg.(*dynamicpb.Message); ok {
			bytes, err := proto.Marshal(dt)
			if err != nil {
				return t, false
			}
			if err := proto.Unmarshal(bytes, t); err != nil {
				return t, false
			}
			return t, true
		}
	}
	return t, false
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
