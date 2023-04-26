package generators

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
)

func getExtension[T proto.Message](desc protoreflect.Descriptor, ext *protoimpl.ExtensionInfo) (out T, ok bool) {
	if proto.HasExtension(desc.Options(), ext) {
		out, ok = proto.GetExtension(desc.Options(), ext).(T)
	}
	return
}

func getFileOptions(file protoreflect.Descriptor) (*GeneratorOptions, bool) {
	return getExtension[*GeneratorOptions](file, E_Generator)
}

func getServiceOptions(svc protoreflect.Descriptor) (*CommandGroupOptions, bool) {
	return getExtension[*CommandGroupOptions](svc, E_CommandGroup)
}

func getMethodOptions(mtd protoreflect.Descriptor) (*CommandOptions, bool) {
	return getExtension[*CommandOptions](mtd, E_Command)
}

func getFieldOptions(fld protoreflect.Descriptor) (*FlagOptions, bool) {
	return getExtension[*FlagOptions](fld, E_Flag)
}

func applyOptions(desc protoreflect.Descriptor, out proto.Message) {
	var opts proto.Message
	var ok bool
	switch desc := desc.(type) {
	case protoreflect.FileDescriptor:
		opts, ok = getFileOptions(desc)
	case protoreflect.ServiceDescriptor:
		opts, ok = getServiceOptions(desc)
	case protoreflect.MethodDescriptor:
		opts, ok = getMethodOptions(desc)
	case protoreflect.FieldDescriptor:
		opts, ok = getFieldOptions(desc)
	}
	if ok {
		proto.Merge(out, opts)
	}
}
