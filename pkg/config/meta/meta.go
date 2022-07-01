package meta

import (
	"errors"
	"reflect"

	"github.com/alecthomas/jsonschema"
)

var ErrUnknownObjectKind = errors.New("unknown object kind")

type TypeMeta struct {
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
}

type ObjectMeta struct {
	Name string `json:"name,omitempty"`
}

type Object interface {
	GetAPIVersion() string
	GetKind() string
}

type NamedObject interface {
	Object
	GetName() string
}

func (t TypeMeta) GetAPIVersion() string {
	return t.APIVersion
}

func (t TypeMeta) GetKind() string {
	return t.Kind
}

func (o ObjectMeta) GetName() string {
	return o.Name
}

type ObjectList []Object

type ObjectVisitorFunc = interface{} // func(*T) or func(*T, *jsonschema.Schema)
func (l ObjectList) Visit(visitors ...ObjectVisitorFunc) (visitedAny bool) {
	// For each object in the list, call each visitor if its argument type
	// matches the concrete type of the object.
	for _, vf := range visitors {
		// get the type of the first arg
		fn := reflect.TypeOf(vf)
		if fn.Kind() != reflect.Func {
			panic("visitor must be a function")
		}
		switch fn.NumIn() {
		case 1: // func(*T)
			t := fn.In(0)
			fn := reflect.ValueOf(vf)
			for _, o := range l {
				if reflect.TypeOf(o) == t {
					visitedAny = true
					fn.Call([]reflect.Value{reflect.ValueOf(o)})
				}
			}
		case 2: // func(*T, *jsonschema.Schema)
			t := fn.In(0)
			s := fn.In(1)
			if s != reflect.TypeOf(&jsonschema.Schema{}) {
				panic("second argument must be of type *jsonschema.Schema")
			}
			fn := reflect.ValueOf(vf)
			var emptyIntf interface{}
			emptyIntfType := reflect.TypeOf(&emptyIntf).Elem()
			for _, o := range l {
				if t == emptyIntfType || reflect.TypeOf(o) == t {
					visitedAny = true
					schema := jsonschema.Reflect(o)
					fn.Call([]reflect.Value{reflect.ValueOf(o), reflect.ValueOf(schema)})
				}
			}
		}
	}
	return
}
