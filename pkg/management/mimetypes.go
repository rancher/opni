package management

import (
	"bytes"
	"fmt"
	"io"

	"github.com/golang/protobuf/jsonpb"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/runtime/protoimpl"
)

type DynamicV1Marshaler struct{}

var _ runtime.Marshaler = (*DynamicV1Marshaler)(nil)

// ContentType implements runtime.Marshaler.
func (*DynamicV1Marshaler) ContentType(_ any) string {
	return "application/octet-stream"
}

// Marshal implements runtime.Marshaler.
func (*DynamicV1Marshaler) Marshal(v any) ([]byte, error) {
	return proto.Marshal(protoimpl.X.ProtoMessageV2Of(v))
}

var unmarshalMerge = proto.UnmarshalOptions{
	Merge: true,
}

// NewDecoder implements runtime.Marshaler.
func (*DynamicV1Marshaler) NewDecoder(r io.Reader) runtime.Decoder {
	return runtime.DecoderFunc(func(v any) error {
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		switch msg := v.(type) {
		case *dynamic.Message:
			return msg.UnmarshalMerge(data)
		case proto.Message:
			return unmarshalMerge.Unmarshal(data, msg)
		default:
			return unmarshalMerge.Unmarshal(data, protoimpl.X.ProtoMessageV2Of(msg))
		}
	})
}

// NewEncoder implements runtime.Marshaler.
func (*DynamicV1Marshaler) NewEncoder(w io.Writer) runtime.Encoder {
	return runtime.EncoderFunc(func(v any) error {
		bytes, err := proto.Marshal(protoimpl.X.ProtoMessageV2Of(v))
		if err != nil {
			return err
		}
		_, err = w.Write(bytes)
		return err
	})
}

// Unmarshal implements runtime.Marshaler.
func (*DynamicV1Marshaler) Unmarshal(data []byte, v any) error {
	switch msg := v.(type) {
	case *dynamic.Message:
		return msg.UnmarshalMerge(data)
	case proto.Message:
		return unmarshalMerge.Unmarshal(data, msg)
	default:
		return unmarshalMerge.Unmarshal(data, protoimpl.X.ProtoMessageV2Of(msg))
	}
}

type LegacyJsonMarshaler struct{}

var _ runtime.Marshaler = (*LegacyJsonMarshaler)(nil)

// ContentType implements runtime.Marshaler.
func (*LegacyJsonMarshaler) ContentType(_ any) string {
	return "application/json"
}

// Marshal implements runtime.Marshaler.
func (*LegacyJsonMarshaler) Marshal(v any) ([]byte, error) {
	m := jsonpb.Marshaler{
		EnumsAsInts:  true,
		EmitDefaults: true,
	}
	var buf bytes.Buffer
	if err := m.Marshal(&buf, v.(protoiface.MessageV1)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// NewDecoder implements runtime.Marshaler.
func (m *LegacyJsonMarshaler) NewDecoder(r io.Reader) runtime.Decoder {
	return runtime.DecoderFunc(func(v any) error {
		bytes, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		return m.Unmarshal(bytes, v)
	})
}

// NewEncoder implements runtime.Marshaler.
func (*LegacyJsonMarshaler) NewEncoder(w io.Writer) runtime.Encoder {
	return runtime.EncoderFunc(func(v any) error {
		m := jsonpb.Marshaler{
			EnumsAsInts:  true,
			EmitDefaults: true,
		}
		return m.Marshal(w, v.(protoiface.MessageV1))
	})
}

// Unmarshal implements runtime.Marshaler.
func (*LegacyJsonMarshaler) Unmarshal(data []byte, v interface{}) error {
	switch msg := v.(type) {
	case *dynamic.Message:
		return msg.UnmarshalMergeJSON(data)
	case protoiface.MessageV1:
		dm, err := dynamic.AsDynamicMessage(msg)
		if err != nil {
			return err
		}
		return dm.UnmarshalMergeJSON(data)
	case proto.Message:
		clone := proto.Clone(msg)
		proto.Reset(msg)
		if err := protojson.Unmarshal(data, msg); err != nil {
			return err
		}
		proto.Merge(msg, clone)
	default:
		panic(fmt.Sprintf("bug: Unmarshal called with unexpected type %T", msg))
	}
	return nil
}
