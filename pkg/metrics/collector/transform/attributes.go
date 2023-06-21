// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transform

// Copied from
// https://github.com/open-telemetry/opentelemetry-go/exporters/otlp/otlpmetric/otlpmetricgrpc/exporters/otlp/otlpmetric/internal/transform/attribute.go
// version v0.39.0

import (
	"go.opentelemetry.io/otel/attribute"
	cpb "go.opentelemetry.io/proto/otlp/common/v1"
)

// AttrIter transforms an attribute iterator into OTLP key-values.
func AttrIter(iter attribute.Iterator) []*cpb.KeyValue {
	l := iter.Len()
	if l == 0 {
		return nil
	}

	out := make([]*cpb.KeyValue, 0, l)
	for iter.Next() {
		out = append(out, KeyValue(iter.Attribute()))
	}
	return out
}

// KeyValues transforms a slice of attribute KeyValues into OTLP key-values.
func KeyValues(attrs []attribute.KeyValue) []*cpb.KeyValue {
	if len(attrs) == 0 {
		return nil
	}

	out := make([]*cpb.KeyValue, 0, len(attrs))
	for _, kv := range attrs {
		out = append(out, KeyValue(kv))
	}
	return out
}

func ProtoToKeyValues(attrs []*cpb.KeyValue) []attribute.KeyValue {
	if len(attrs) == 0 {
		return nil
	}

	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, kv := range attrs {
		out = append(out, ProtoToKeyValue(kv))
	}
	return out
}

// KeyValue transforms an attribute KeyValue into an OTLP key-value.
func KeyValue(kv attribute.KeyValue) *cpb.KeyValue {
	return &cpb.KeyValue{Key: string(kv.Key), Value: Value(kv.Value)}
}

// ProtoToKeyValue transforms an OTLP key-value into an attribute KeyValue.
func ProtoToKeyValue(kv *cpb.KeyValue) attribute.KeyValue {
	return attribute.KeyValue{
		Key:   attribute.Key(kv.GetKey()),
		Value: ProtoToValue(kv.GetValue()),
	}
}

// Value transforms an attribute Value into an OTLP AnyValue.
func Value(v attribute.Value) *cpb.AnyValue {
	av := new(cpb.AnyValue)
	switch v.Type() {
	case attribute.BOOL:
		av.Value = &cpb.AnyValue_BoolValue{
			BoolValue: v.AsBool(),
		}
	case attribute.BOOLSLICE:
		av.Value = &cpb.AnyValue_ArrayValue{
			ArrayValue: &cpb.ArrayValue{
				Values: boolSliceValues(v.AsBoolSlice()),
			},
		}
	case attribute.INT64:
		av.Value = &cpb.AnyValue_IntValue{
			IntValue: v.AsInt64(),
		}
	case attribute.INT64SLICE:
		av.Value = &cpb.AnyValue_ArrayValue{
			ArrayValue: &cpb.ArrayValue{
				Values: int64SliceValues(v.AsInt64Slice()),
			},
		}
	case attribute.FLOAT64:
		av.Value = &cpb.AnyValue_DoubleValue{
			DoubleValue: v.AsFloat64(),
		}
	case attribute.FLOAT64SLICE:
		av.Value = &cpb.AnyValue_ArrayValue{
			ArrayValue: &cpb.ArrayValue{
				Values: float64SliceValues(v.AsFloat64Slice()),
			},
		}
	case attribute.STRING:
		av.Value = &cpb.AnyValue_StringValue{
			StringValue: v.AsString(),
		}
	case attribute.STRINGSLICE:
		av.Value = &cpb.AnyValue_ArrayValue{
			ArrayValue: &cpb.ArrayValue{
				Values: stringSliceValues(v.AsStringSlice()),
			},
		}
	default:
		av.Value = &cpb.AnyValue_StringValue{
			StringValue: "INVALID",
		}
	}
	return av
}

// ProtoToValue transforms an OTLP AnyValue into an attribute Value.
func ProtoToValue(val *cpb.AnyValue) attribute.Value {
	switch val.Value.(type) {
	case *cpb.AnyValue_BoolValue:
		return attribute.BoolValue(val.GetBoolValue())
	case *cpb.AnyValue_IntValue:
		return attribute.Int64Value(val.GetIntValue())
	case *cpb.AnyValue_DoubleValue:
		return attribute.Float64Value(val.GetDoubleValue())
	case *cpb.AnyValue_StringValue:
		return attribute.StringValue(val.GetStringValue())
	case *cpb.AnyValue_ArrayValue:
		return ProtoToValueArray(val.GetArrayValue())
	default:
		return attribute.StringValue("INVALID")
	}
}

func ProtoToValueArray(vals *cpb.ArrayValue) attribute.Value {
	values := vals.GetValues()
	if len(values) == 0 {
		return attribute.StringSliceValue([]string{})
	}
	val := values[0]
	switch val.Value.(type) {
	case *cpb.AnyValue_BoolValue:
		return attribute.BoolSliceValue(valuesToBoolSlice(val.GetArrayValue().GetValues()))
	case *cpb.AnyValue_IntValue:
		return attribute.Int64SliceValue(valuesToInt64Slice(val.GetArrayValue().GetValues()))
	case *cpb.AnyValue_DoubleValue:
		return attribute.Float64SliceValue(valuesToFloat64Slice(val.GetArrayValue().GetValues()))
	case *cpb.AnyValue_StringValue:
		return attribute.StringSliceValue(valuesToStringSlice(val.GetArrayValue().GetValues()))
	default:
		return attribute.StringSliceValue([]string{"INVALID"})
	}
}

func boolSliceValues(vals []bool) []*cpb.AnyValue {
	converted := make([]*cpb.AnyValue, len(vals))
	for i, v := range vals {
		converted[i] = &cpb.AnyValue{
			Value: &cpb.AnyValue_BoolValue{
				BoolValue: v,
			},
		}
	}
	return converted
}

func valuesToBoolSlice(vals []*cpb.AnyValue) []bool {
	converted := make([]bool, len(vals))
	for i, v := range vals {
		converted[i] = v.GetBoolValue()
	}
	return converted
}

func int64SliceValues(vals []int64) []*cpb.AnyValue {
	converted := make([]*cpb.AnyValue, len(vals))
	for i, v := range vals {
		converted[i] = &cpb.AnyValue{
			Value: &cpb.AnyValue_IntValue{
				IntValue: v,
			},
		}
	}
	return converted
}

func valuesToInt64Slice(vals []*cpb.AnyValue) []int64 {
	converted := make([]int64, len(vals))
	for i, v := range vals {
		converted[i] = v.GetIntValue()
	}
	return converted
}

func float64SliceValues(vals []float64) []*cpb.AnyValue {
	converted := make([]*cpb.AnyValue, len(vals))
	for i, v := range vals {
		converted[i] = &cpb.AnyValue{
			Value: &cpb.AnyValue_DoubleValue{
				DoubleValue: v,
			},
		}
	}
	return converted
}

func valuesToFloat64Slice(vals []*cpb.AnyValue) []float64 {
	converted := make([]float64, len(vals))
	for i, v := range vals {
		converted[i] = v.GetDoubleValue()
	}
	return converted
}

func stringSliceValues(vals []string) []*cpb.AnyValue {
	converted := make([]*cpb.AnyValue, len(vals))
	for i, v := range vals {
		converted[i] = &cpb.AnyValue{
			Value: &cpb.AnyValue_StringValue{
				StringValue: v,
			},
		}
	}
	return converted
}

func valuesToStringSlice(vals []*cpb.AnyValue) []string {
	converted := make([]string, len(vals))
	for i, v := range vals {
		converted[i] = v.GetStringValue()
	}
	return converted
}
