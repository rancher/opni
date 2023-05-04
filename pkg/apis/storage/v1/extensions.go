package v1

import (
	"encoding/json"
	"fmt"
	"reflect"
)

func (b *Backend) UnmarshalJSON(data []byte) error {
	var backend string
	if err := json.Unmarshal(data, &backend); err != nil {
		return err
	}
	if v, ok := Backend_value[backend]; ok {
		*b = Backend(v)
	} else {
		return &json.UnsupportedValueError{
			Value: reflect.ValueOf(backend),
			Str:   fmt.Sprintf("unknown value %q for enum %T", backend, *b),
		}
	}
	return nil
}

func (b Backend) MarshalJSON() ([]byte, error) {
	str := Backend_name[int32(b)]
	if str == "" {
		return nil, fmt.Errorf("unknown value %d for enum %T", b, b)
	}
	return json.Marshal(str)
}
