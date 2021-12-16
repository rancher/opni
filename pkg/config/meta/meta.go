package meta

import (
	"errors"
	"reflect"
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

type ObjectVisitorFunc = interface{} // func(*T)
func (l ObjectList) Visit(visitors ...ObjectVisitorFunc) {
	// For each object in the list, call each visitor if its argument type
	// matches the concrete type of the object.
	for _, vf := range visitors {
		// get the type of the first arg
		t := reflect.TypeOf(vf).In(0)
		fn := reflect.ValueOf(vf)
		for _, o := range l {
			if reflect.TypeOf(o) == t {
				fn.Call([]reflect.Value{reflect.ValueOf(o)})
			}
		}
	}
}
