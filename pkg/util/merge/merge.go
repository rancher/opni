// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains the implementation of proto.Merge, with new merge options.
package merge

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Merge merges src into dst, which must be a message with the same descriptor.
//
// Populated scalar fields in src are copied to dst, while populated
// singular messages in src are merged into dst by recursively calling Merge.
// The elements of every list field in src is appended to the corresponded
// list fields in dst, unless the ReplaceLists option is true, in which case the
// list fields in dst are replaced entirely. The entries of every map field in
// src is copied into the corresponding map field in dst, possibly replacing
// existing entries. The unknown fields of src are appended to the unknown
// fields of dst.
//
// It is semantically equivalent to unmarshaling the encoded form of src
// into dst with the UnmarshalOptions.Merge option specified.
func Merge(dst, src proto.Message) {
	MergeOptions{}.Merge(dst, src)
}

func MergeWithReplace[T proto.Message](dst, src T) {
	MergeOptions{
		ReplaceLists: true,
		ReplaceMaps:  true,
	}.Merge(dst, src)
}

// MergeOptions provides a namespace for merge functions, and can be
// exported in the future if we add user-visible merge options.
type MergeOptions struct {
	// If true, slices are replaced instead of appended to.
	ReplaceLists bool

	// If true, maps are replaced instead of merged.
	ReplaceMaps bool
}

func (o MergeOptions) Merge(dst, src proto.Message) {
	dstMsg, srcMsg := dst.ProtoReflect(), src.ProtoReflect()
	if dstMsg.Descriptor() != srcMsg.Descriptor() {
		if got, want := dstMsg.Descriptor().FullName(), srcMsg.Descriptor().FullName(); got != want {
			panic(fmt.Sprintf("descriptor mismatch: %v != %v", got, want))
		}
		panic("descriptor mismatch")
	}
	o.mergeMessage(dstMsg, srcMsg)
}

func (o MergeOptions) mergeMessage(dst, src protoreflect.Message) {
	if o == (MergeOptions{}) {
		// Can only use optimized merge if there are no options set.
		methods := dst.ProtoMethods()
		if methods != nil && methods.Merge != nil {
			in := protoiface.MergeInput{
				Destination: dst,
				Source:      src,
			}
			out := methods.Merge(in)
			if out.Flags&protoiface.MergeComplete != 0 {
				return
			}
		}
	}

	if fqn := src.Descriptor().FullName(); fqn == "google.protobuf.Duration" {
		srcpb := src.Interface().(*durationpb.Duration)
		dstpb := dst.Interface().(*durationpb.Duration)
		if srcpb != nil && dstpb != nil && srcpb.Seconds == 0 && srcpb.Nanos == 0 {
			dstpb.Seconds = 0
			dstpb.Nanos = 0
			return
		}
	} else if fqn == "google.protobuf.Timestamp" {
		srcpb := src.Interface().(*timestamppb.Timestamp)
		dstpb := dst.Interface().(*timestamppb.Timestamp)
		if srcpb != nil && dstpb != nil && srcpb.Seconds == 0 && srcpb.Nanos == 0 {
			dstpb.Seconds = 0
			dstpb.Nanos = 0
			return
		}
	}

	if !dst.IsValid() {
		panic(fmt.Sprintf("cannot merge into nil %v message", dst.Descriptor().FullName()))
	}

	src.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList():
			o.mergeList(dst.Mutable(fd).List(), v.List(), fd)
		case fd.IsMap():
			o.mergeMap(dst.Mutable(fd).Map(), v.Map(), fd.MapValue())
		case fd.Message() != nil:
			o.mergeMessage(dst.Mutable(fd).Message(), v.Message())
		case fd.Kind() == protoreflect.BytesKind:
			dst.Set(fd, o.cloneBytes(v))
		default:
			dst.Set(fd, v)
		}
		return true
	})

	if len(src.GetUnknown()) > 0 {
		dst.SetUnknown(append(dst.GetUnknown(), src.GetUnknown()...))
	}
}

func (o MergeOptions) mergeList(dst, src protoreflect.List, fd protoreflect.FieldDescriptor) {
	if o.ReplaceLists {
		dst.Truncate(0)
	}
	for i, n := 0, src.Len(); i < n; i++ {
		switch v := src.Get(i); {
		case fd.Message() != nil:
			dstv := dst.NewElement()
			o.mergeMessage(dstv.Message(), v.Message())
			dst.Append(dstv)
		case fd.Kind() == protoreflect.BytesKind:
			dst.Append(o.cloneBytes(v))
		default:
			dst.Append(v)
		}
	}
}

func (o MergeOptions) mergeMap(dst, src protoreflect.Map, fd protoreflect.FieldDescriptor) {
	if o.ReplaceMaps {
		dst.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			dst.Clear(k)
			return true
		})
	}
	src.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
		switch {
		case fd.Message() != nil:
			dstv := dst.NewValue()
			o.mergeMessage(dstv.Message(), v.Message())
			dst.Set(k, dstv)
		case fd.Kind() == protoreflect.BytesKind:
			dst.Set(k, o.cloneBytes(v))
		default:
			dst.Set(k, v)
		}
		return true
	})
}

func (o MergeOptions) cloneBytes(v protoreflect.Value) protoreflect.Value {
	return protoreflect.ValueOfBytes(append([]byte{}, v.Bytes()...))
}
