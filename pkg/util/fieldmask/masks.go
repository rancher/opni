package fieldmask

import (
	"fmt"

	art "github.com/plar/go-adaptive-radix-tree"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func ByPresence[T protoreflect.Message](msg T) *fieldmaskpb.FieldMask {
	mask := &fieldmaskpb.FieldMask{
		Paths: rangePresentMessageFields(msg),
	}
	mask.Normalize()
	return mask
}

func ByAbsence[T protoreflect.Message](msg T) *fieldmaskpb.FieldMask {
	mask := &fieldmaskpb.FieldMask{
		Paths: rangeAbsentMessageFields(msg),
	}
	mask.Normalize()
	return mask
}

func AllFields[T proto.Message]() *fieldmaskpb.FieldMask {
	var t T
	desc := t.ProtoReflect().Descriptor()
	mask := &fieldmaskpb.FieldMask{
		Paths: rangeDescriptorFields(desc),
	}
	mask.Normalize()
	return mask
}

func rangeDescriptorFields(desc protoreflect.MessageDescriptor) []string {
	fields := desc.Fields()
	paths := []string{}
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		kind := field.Kind()
		if kind == protoreflect.MessageKind && !field.IsMap() && !field.IsList() {
			nestedPaths := rangeDescriptorFields(field.Message())
			for _, nestedPath := range nestedPaths {
				paths = append(paths, fmt.Sprintf("%s.%s", field.Name(), nestedPath))
			}
		} else {
			paths = append(paths, string(field.Name()))
		}
	}
	return paths
}

func rangePresentMessageFields(msg protoreflect.Message) []string {
	fields := msg.Descriptor().Fields()
	paths := []string{}
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		kind := field.Kind()
		if !msg.Has(field) {
			continue
		}
		if kind == protoreflect.MessageKind && !field.IsMap() && !field.IsList() {
			nestedPaths := rangePresentMessageFields(msg.Get(field).Message())
			for _, nestedPath := range nestedPaths {
				paths = append(paths, fmt.Sprintf("%s.%s", field.Name(), nestedPath))
			}
		} else {
			paths = append(paths, string(field.Name()))
		}
	}
	return paths
}

func rangeAbsentMessageFields(msg protoreflect.Message) []string {
	fields := msg.Descriptor().Fields()
	paths := []string{}
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		kind := field.Kind()
		if !msg.Has(field) {
			paths = append(paths, string(field.Name()))
			continue
		}
		if kind == protoreflect.MessageKind && !field.IsMap() && !field.IsList() {
			nestedPaths := rangeAbsentMessageFields(msg.Get(field).Message())
			for _, nestedPath := range nestedPaths {
				paths = append(paths, fmt.Sprintf("%s.%s", field.Name(), nestedPath))
			}
		}
	}
	return paths
}

// Recursively clears all fields except those listed in the mask, such that:
// 1. ExclusiveKeep(msg, ByPresence(msg)) == msg
// 2. ExclusiveKeep(msg, ByAbsence(msg)) == &T{}
//
// A nil mask is a no-op, and is not the same as a non-nil, empty mask.
func ExclusiveKeep(msg proto.Message, mask *fieldmaskpb.FieldMask) {
	if mask == nil {
		return
	}
	keep(msg.ProtoReflect(), "", AsTree(mask))
}

// Recursively clears all fields except those NOT listed in the mask, such that:
// 1. ExclusiveDiscard(msg, ByPresence(msg)) == &T{}
// 2. ExclusiveDiscard(msg, ByAbsence(msg)) == msg
//
// A nil mask is a no-op, and is not the same as a non-nil, empty mask.
func ExclusiveDiscard(msg proto.Message, mask *fieldmaskpb.FieldMask) {
	if mask == nil {
		return
	}
	discard(msg.ProtoReflect(), "", AsTree(mask))
}

func keep(msg protoreflect.Message, prefix string, tree art.Tree) (keptAny bool) {
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		path := prefix + string(field.Name())
		msgHasField := msg.Has(field)
		if !msgHasField {
			continue
		}
		maskHasFieldOrPrefix := false
		tree.ForEachPrefix(art.Key(path), func(node art.Node) bool {
			if node.Kind() != art.Leaf {
				return true
			}
			maskHasFieldOrPrefix = true
			return false
		})
		if maskHasFieldOrPrefix {
			if field.Kind() == protoreflect.MessageKind && !field.IsMap() && !field.IsList() {
				// if the field is an exact match (not a prefix) of a message, we need
				// to keep the entire field regardless of its contents.
				_, maskHasExactField := tree.Search(art.Key(path))
				if !maskHasExactField && !keep(msg.Mutable(field).Message(), path+".", tree) {
					msg.Clear(field)
					continue
				}
			}
			keptAny = true
			continue
		}

		msg.Clear(field)
	}
	return
}

func discard(msg protoreflect.Message, prefix string, tree art.Tree) (keptAny bool) {
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		path := prefix + string(field.Name())
		msgHasField := msg.Has(field)
		if !msgHasField {
			continue
		}
		maskHasFieldOrPrefix := false
		tree.ForEachPrefix(art.Key(path), func(node art.Node) bool {
			if node.Kind() != art.Leaf {
				return true
			}
			maskHasFieldOrPrefix = true
			return false
		})
		if !maskHasFieldOrPrefix {
			keptAny = true
			continue
		}
		if field.Kind() == protoreflect.MessageKind && !field.IsMap() && !field.IsList() {
			// if the field is an exact match (not a prefix) of a message, we need
			// to clear the entire field regardless of its contents.
			_, maskHasExactField := tree.Search(art.Key(path))
			if maskHasExactField || !discard(msg.Mutable(field).Message(), path+".", tree) {
				msg.Clear(field)
				continue
			}
			keptAny = true
			continue
		}

		msg.Clear(field)
	}
	return
}

func AsTree(mask *fieldmaskpb.FieldMask) art.Tree {
	tree := art.New()
	for _, path := range mask.GetPaths() {
		tree.Insert(art.Key(path), struct{}{})
	}
	return tree
}
