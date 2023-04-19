package driverutil

import (
	"errors"
	"fmt"
	"reflect"
)

type option[T any] struct {
	key   string
	value T
}

type Option interface {
	Apply(dest any) error
}

func ApplyOptions(dest any, opts ...Option) error {
	var errs []error
	for _, opt := range opts {
		if err := opt.Apply(dest); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (o *option[T]) Apply(dest any) error {
	if o.key == "" {
		return nil
	}
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr {
		panic("destination must be a pointer")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		panic("destination be a pointer to a struct")
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}
		tag := v.Type().Field(i).Tag.Get("option")
		if tag == o.key {
			val := reflect.ValueOf(o.value)
			if val.IsZero() {
				return nil
			}
			if field.Type() != reflect.TypeOf(o.value) {
				return fmt.Errorf("mismatched option types for key %q: %T != %T", tag, field.Interface(), o.value)
			}
			field.Set(val)
			return nil
		}
	}

	return nil
}

func NewOption[T any](key string, value T) Option {
	return &option[T]{
		key:   key,
		value: value,
	}
}
