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
	SharedKeys   []*SharedKeys   `json:"sharedKeys,omitempty"`
	PKPKey       []*PKPKey       `json:"pkpKey,omitempty"`
	CACertsKey   []*CACertsKey   `json:"caCertsKey,omitempty"`
	EphemeralKey []*EphemeralKey `json:"-"`
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

type UseKeyFn any // func(*KeyringType)

type Keyring interface {
	// Try will call the provided function for each key matching the type of
	// the argument of the function. Try intentionally does not short-circuit;
	// it will always call the function for every key.
	Try(...UseKeyFn) bool
	// ForEach is like Try, except it will call the function for every key
	// regardless of type.
	ForEach(func(key any))
	// Marshal returns a wire representation of the keyring. The specific format
	// is unspecified.
	Marshal() ([]byte, error)
	// Merge returns a new keyring that contains all keys from both keyrings.
	// It does not deduplicate keys.
	Merge(Keyring) Keyring
}

type keyring struct {
	Keys map[reflect.Type][]any
}

func New(keys ...any) Keyring {
	m := map[reflect.Type][]any{}
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
	keys := []any{}
	kr.ForEach(func(key any) {
		keys = append(keys, key)
	})
	other.ForEach(func(key any) {
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

func (kr *keyring) ForEach(fn func(key any)) {
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
	values := []any{}
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
