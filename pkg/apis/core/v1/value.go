package v1

import (
	"fmt"
	"slices"

	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	anypb "google.golang.org/protobuf/types/known/anypb"
)

func NewValue(in protoreflect.Value) *Value {
	v := &Value{}
	if !in.IsValid() {
		return v
	}
	switch iv := in.Interface().(type) {
	case bool:
		v.Data = &Value_BoolValue{BoolValue: iv}
	case int32:
		v.Data = &Value_Int32Value{Int32Value: iv}
	case int64:
		v.Data = &Value_Int64Value{Int64Value: iv}
	case uint32:
		v.Data = &Value_Uint32Value{Uint32Value: iv}
	case uint64:
		v.Data = &Value_Uint64Value{Uint64Value: iv}
	case float32:
		v.Data = &Value_Float32Value{Float32Value: iv}
	case float64:
		v.Data = &Value_Float64Value{Float64Value: iv}
	case string:
		v.Data = &Value_StringValue{StringValue: iv}
	case []byte:
		v.Data = &Value_BytesValue{BytesValue: iv}
	case protoreflect.EnumNumber:
		v.Data = &Value_Enum{Enum: int32(iv)}
	case protoreflect.Message:
		a, err := anypb.New(iv.Interface())
		if err != nil {
			panic(err)
		}
		v.Data = &Value_Message{Message: a}
	case protoreflect.List:
		values := make([]*Value, 0, iv.Len())
		for i, l := 0, iv.Len(); i < l; i++ {
			values = append(values, NewValue(iv.Get(i)))
		}
		v.Data = &Value_List{
			List: &Value_ListValue{
				Values: values,
			},
		}
	case protoreflect.Map:
		var entries []*Value_MapEntry
		iv.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			var key *Value_MapKey
			switch k := k.Interface().(type) {
			case bool:
				key = &Value_MapKey{Data: &Value_MapKey_BoolValue{BoolValue: k}}
			case int32:
				key = &Value_MapKey{Data: &Value_MapKey_Int32Value{Int32Value: k}}
			case int64:
				key = &Value_MapKey{Data: &Value_MapKey_Int64Value{Int64Value: k}}
			case uint32:
				key = &Value_MapKey{Data: &Value_MapKey_Uint32Value{Uint32Value: k}}
			case uint64:
				key = &Value_MapKey{Data: &Value_MapKey_Uint64Value{Uint64Value: k}}
			case string:
				key = &Value_MapKey{Data: &Value_MapKey_StringValue{StringValue: k}}
			default:
				panic(fmt.Sprintf("bug: invalid map key type: %T", k))
			}
			entries = append(entries, &Value_MapEntry{
				Key:   key,
				Value: NewValue(v),
			})
			return true
		})
		v.Data = &Value_Map{
			Map: &Value_MapValue{
				Entries: entries,
			},
		}
	}
	return v
}

func (v *Value) ToValue() protoreflect.Value {
	if v == nil || v.Data == nil {
		return protoreflect.Value{}
	}

	switch data := v.Data.(type) {
	case *Value_BoolValue:
		return protoreflect.ValueOfBool(data.BoolValue)
	case *Value_Int32Value:
		return protoreflect.ValueOfInt32(data.Int32Value)
	case *Value_Int64Value:
		return protoreflect.ValueOfInt64(data.Int64Value)
	case *Value_Uint32Value:
		return protoreflect.ValueOfUint32(data.Uint32Value)
	case *Value_Uint64Value:
		return protoreflect.ValueOfUint64(data.Uint64Value)
	case *Value_Float32Value:
		return protoreflect.ValueOfFloat32(data.Float32Value)
	case *Value_Float64Value:
		return protoreflect.ValueOfFloat64(data.Float64Value)
	case *Value_StringValue:
		return protoreflect.ValueOfString(data.StringValue)
	case *Value_BytesValue:
		return protoreflect.ValueOfBytes(data.BytesValue)
	case *Value_Enum:
		return protoreflect.ValueOfEnum(protoreflect.EnumNumber(data.Enum))
	case *Value_Message:
		msg, err := data.Message.UnmarshalNew()
		if err != nil {
			panic(err)
		}
		return protoreflect.ValueOfMessage(msg.ProtoReflect())
	case *Value_List:
		lv := &listValue{}
		lv.list = slices.Grow(lv.list, len(data.List.Values))
		for _, v := range data.List.Values {
			lv.Append(v.ToValue())
		}
		return protoreflect.ValueOfList(lv)
	case *Value_Map:
		mv := &mapValue{}
		mv.mapv = make(map[any]protoreflect.Value, len(data.Map.Entries))
		for _, e := range data.Map.Entries {
			mv.Set(e.Key.ToMapKey(), e.Value.ToValue())
		}
		return protoreflect.ValueOfMap(mv)
	default:
		panic(fmt.Sprintf("bug: invalid value type: %T", v.Data))
	}
}

func (mk *Value_MapKey) ToMapKey() protoreflect.MapKey {
	switch data := mk.Data.(type) {
	case *Value_MapKey_BoolValue:
		return protoreflect.ValueOfBool(data.BoolValue).MapKey()
	case *Value_MapKey_Int32Value:
		return protoreflect.ValueOfInt32(data.Int32Value).MapKey()
	case *Value_MapKey_Int64Value:
		return protoreflect.ValueOfInt64(data.Int64Value).MapKey()
	case *Value_MapKey_Uint32Value:
		return protoreflect.ValueOfUint32(data.Uint32Value).MapKey()
	case *Value_MapKey_Uint64Value:
		return protoreflect.ValueOfUint64(data.Uint64Value).MapKey()
	case *Value_MapKey_StringValue:
		return protoreflect.ValueOfString(data.StringValue).MapKey()
	}
	panic(fmt.Sprintf("bug: invalid map key type: %T", mk))
}

type listValue struct {
	list []protoreflect.Value
}

var _ protoreflect.List = (*listValue)(nil)

func (l *listValue) Len() int {
	return len(l.list)
}

func (l *listValue) Get(i int) protoreflect.Value {
	return l.list[i]
}

func (l *listValue) Set(i int, value protoreflect.Value) {
	l.list[i] = value
}

func (l *listValue) Append(value protoreflect.Value) {
	l.list = append(l.list, value)
}

func (l *listValue) AppendMutable() protoreflect.Value {
	panic("AppendMutable not supported")
}

func (l *listValue) Truncate(i int) {
	l.list = l.list[:i]
}

func (l *listValue) NewElement() protoreflect.Value {
	panic("NewElement not supported")
}

func (l *listValue) IsValid() bool {
	return true
}

type mapValue struct {
	mapv map[any]protoreflect.Value
}

// Clear implements protoreflect.Map.
func (m *mapValue) Clear(k protoreflect.MapKey) {
	delete(m.mapv, k.Interface())
}

// Get implements protoreflect.Map.
func (m *mapValue) Get(k protoreflect.MapKey) protoreflect.Value {
	return m.mapv[k.Interface()]
}

// Has implements protoreflect.Map.
func (m *mapValue) Has(k protoreflect.MapKey) bool {
	return m.Get(k).IsValid()
}

// IsValid implements protoreflect.Map.
func (m *mapValue) IsValid() bool {
	return m.mapv != nil
}

// Len implements protoreflect.Map.
func (m *mapValue) Len() int {
	return len(m.mapv)
}

// Mutable implements protoreflect.Map.
func (m *mapValue) Mutable(protoreflect.MapKey) protoreflect.Value {
	panic("Mutable not supported")
}

// NewValue implements protoreflect.Map.
func (m *mapValue) NewValue() protoreflect.Value {
	panic("NewValue not supported")
}

// Range implements protoreflect.Map.
func (m *mapValue) Range(f func(protoreflect.MapKey, protoreflect.Value) bool) {
	for k, v := range m.mapv {
		if !f(protoreflect.ValueOf(k).MapKey(), v) {
			break
		}
	}
}

// Set implements protoreflect.Map.
func (m *mapValue) Set(k protoreflect.MapKey, v protoreflect.Value) {
	m.mapv[k.Interface()] = v
}

var _ protoreflect.Map = (*mapValue)(nil)
