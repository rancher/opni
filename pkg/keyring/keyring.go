package keyring

import (
	"encoding/json"
	"errors"
	"reflect"
)

var (
	ErrInvalidKeyType = errors.New("invalid key type")
)

var allowedKeyTypes = map[reflect.Type]struct{}{}

type completeKeyring struct {
	SharedKeys []*SharedKeys `json:"sharedKeys,omitempty"`
	PKPKey     []*PKPKey     `json:"pkpKey,omitempty"`
	CACertsKey []*CACertsKey `json:"caCertsKey,omitempty"`
}

func init() {
	complete := completeKeyring{}
	fields := reflect.TypeOf(complete).NumField()
	for i := 0; i < fields; i++ {
		field := reflect.TypeOf(complete).Field(i)
		fieldType := field.Type.Elem()
		allowedKeyTypes[fieldType] = struct{}{}
	}
}

type UseKeyFn interface{} // func(*KeyringType)

type Keyring interface {
	Try(...UseKeyFn) bool
	ForEach(func(key interface{}))
	Marshal() ([]byte, error)
	Merge(Keyring) Keyring
}

type keyring struct {
	Keys map[reflect.Type][]interface{}
}

func New(keys ...interface{}) Keyring {
	m := map[reflect.Type][]interface{}{}
	for _, key := range keys {
		t := reflect.TypeOf(key)
		if _, ok := allowedKeyTypes[t]; !ok {
			panic(ErrInvalidKeyType)
		}
		m[t] = append(m[t], key)
	}
	return &keyring{
		Keys: m,
	}
}

func (kr *keyring) Merge(other Keyring) Keyring {
	keys := []interface{}{}
	kr.ForEach(func(key interface{}) {
		keys = append(keys, key)
	})
	other.ForEach(func(key interface{}) {
		keys = append(keys, key)
	})
	return New(keys...)
}

func (kr *keyring) Try(fns ...UseKeyFn) bool {
	found := false
	for _, fn := range fns {
		fnValue := reflect.ValueOf(fn)
		fnType := reflect.TypeOf(fn)
		if fnType.Kind() != reflect.Func {
			panic("invalid UseKeyFn")
		}
		if fnType.NumIn() != 1 {
			panic("invalid UseKeyFn (requires one parameter)")
		}
		argType := fnType.In(0)
		if f, ok := kr.Keys[argType]; ok {
			found = true
			for _, k := range f {
				fnValue.Call([]reflect.Value{reflect.ValueOf(k)})
			}
		}
	}
	return found
}

func (kr *keyring) ForEach(fn func(key interface{})) {
	for _, l := range kr.Keys {
		for _, k := range l {
			fn(k)
		}
	}
}

func (kr *keyring) Marshal() ([]byte, error) {
	complete := completeKeyring{}
	completePtr := reflect.ValueOf(&complete)
	fields := reflect.TypeOf(complete).NumField()
	for i := 0; i < fields; i++ {
		field := reflect.TypeOf(complete).Field(i)
		fieldType := field.Type.Elem()
		if f, ok := kr.Keys[fieldType]; ok {
			keys := reflect.ValueOf(f)
			for j := 0; j < keys.Len(); j++ {
				a := reflect.Append(completePtr.Elem().Field(i), reflect.ValueOf(keys.Index(j).Interface()))
				completePtr.Elem().Field(i).Set(a)
			}
		}
	}
	return json.Marshal(complete)
}

func Unmarshal(data []byte) (Keyring, error) {
	complete := completeKeyring{}
	if err := json.Unmarshal(data, &complete); err != nil {
		return nil, err
	}
	completePtr := reflect.ValueOf(&complete)
	fields := reflect.TypeOf(complete).NumField()
	values := []interface{}{}
	for i := 0; i < fields; i++ {
		field := completePtr.Elem().Field(i)
		if !field.IsNil() && field.Kind() == reflect.Slice {
			for j := 0; j < field.Len(); j++ {
				values = append(values, field.Index(j).Interface())
			}
		}
	}
	return New(values...), nil
}
