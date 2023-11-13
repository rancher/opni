package fieldmask

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// Diff returns a FieldMask representing the fields that have changed between the old and new message.
// For repeated and map fields, the entire field is considered changed if any element has changed.
// Nested messages will be recursively compared to obtain a more fine-grained path if possible.
func Diff[T proto.Message](old, new T) *fieldmaskpb.FieldMask {
	oldReflect, newReflect := old.ProtoReflect(), new.ProtoReflect()
	if oldReflect.Descriptor() != newReflect.Descriptor() {
		panic("bug: cannot compare messages with different descriptors")
	}

	if !oldReflect.IsValid() && !newReflect.IsValid() {
		// Both messages are nil, so there are no changed fields.
		return nil
	}

	var paths []string
	diffFields(oldReflect, newReflect, "", &paths)

	mask := &fieldmaskpb.FieldMask{Paths: paths}
	mask.Normalize()
	return mask
}

func diffFields(oldMsg, newMsg protoreflect.Message, prefix string, paths *[]string) {
	fields := oldMsg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		hasOld, hasNew := oldMsg.Has(field), newMsg.Has(field)
		switch {
		case !hasOld && !hasNew:
			continue
		case hasOld != hasNew:
			// Add field to paths if the field has been added or removed
			*paths = append(*paths, prefix+string(field.Name()))
			continue
		}
		oldValue, newValue := oldMsg.Get(field), newMsg.Get(field)

		if field.Kind() == protoreflect.MessageKind && !field.IsMap() && !field.IsList() {
			// Recursively check nested message fields
			nestedPrefix := prefix + string(field.Name()) + "."
			diffFields(oldValue.Message(), newValue.Message(), nestedPrefix, paths)
		} else if !oldValue.Equal(newValue) {
			// Add field to paths if the value has changed
			*paths = append(*paths, prefix+string(field.Name()))
		}
	}
}
