package configutil

import (
	"io"
	"net/url"
	"reflect"

	kyamlv3 "github.com/kralicky/yaml/v3"
	amcfg "github.com/prometheus/alertmanager/config"
	commoncfg "github.com/prometheus/common/config"
	"github.com/samber/lo"
)

type overrideMarshaler[T kyamlv3.Marshaler] struct {
	fn func(T) (interface{}, error)
}

func (m *overrideMarshaler[T]) MarshalYAML(v interface{}) (interface{}, error) {
	return m.fn(v.(T))
}

func newOverrideMarshaler[T kyamlv3.Marshaler](fn func(T) (interface{}, error)) *overrideMarshaler[T] {
	return &overrideMarshaler[T]{
		fn: fn,
	}
}

func NewSecretOverrideEncoder(w io.Writer) *kyamlv3.Encoder {
	encoder := kyamlv3.NewEncoder(w)
	encoder.OverrideMarshalerForType(
		reflect.TypeOf(amcfg.Secret("")),
		newOverrideMarshaler(func(s amcfg.Secret) (any, error) {
			return string(s), nil
		}),
	)
	encoder.OverrideMarshalerForType(
		reflect.TypeOf(lo.ToPtr(amcfg.Secret(""))),
		newOverrideMarshaler(func(s *amcfg.Secret) (any, error) {
			if s == nil {
				return "", nil
			}
			return string(*s), nil
		}),
	)
	encoder.OverrideMarshalerForType(
		reflect.TypeOf(commoncfg.Secret("")),
		newOverrideMarshaler(func(s commoncfg.Secret) (any, error) {
			return string(s), nil
		}),
	)
	encoder.OverrideMarshalerForType(
		reflect.TypeOf(lo.ToPtr(commoncfg.Secret(""))),
		newOverrideMarshaler(func(s *commoncfg.Secret) (any, error) {
			if s == nil {
				return "", nil
			}
			return string(*s), nil
		}),
	)
	// TODO : check this
	// encoder.OverrideMarshalerForType(
	// 	reflect.TypeOf(commoncfg.SecretURL("")),
	// 	newOverrideMarshaler(func(s commoncfg.SecretURL) (any, error) {
	// 		var u url.URL
	// 		u = url.URL(s)
	// 		return yaml.Marshal(u)
	// 	}),
	// )
	encoder.OverrideMarshalerForType(
		reflect.TypeOf(amcfg.SecretURL{}),
		newOverrideMarshaler(func(s amcfg.SecretURL) (any, error) {
			u := url.URL{
				Scheme:      s.Scheme,
				Opaque:      s.Opaque,
				User:        s.User,
				Host:        s.Host,
				Path:        s.Path,
				RawPath:     s.RawPath,
				OmitHost:    s.OmitHost,
				ForceQuery:  s.ForceQuery,
				RawQuery:    s.RawQuery,
				Fragment:    s.Fragment,
				RawFragment: s.RawFragment,
			}
			b, err := u.MarshalBinary()
			if err != nil {
				return string(b), err
			}
			return string(b), nil
		}),
	)
	encoder.OverrideMarshalerForType(
		reflect.TypeOf(&amcfg.SecretURL{}),
		newOverrideMarshaler(func(s *amcfg.SecretURL) (any, error) {
			u := &url.URL{
				Scheme:      s.Scheme,
				Opaque:      s.Opaque,
				User:        s.User,
				Host:        s.Host,
				Path:        s.Path,
				RawPath:     s.RawPath,
				OmitHost:    s.OmitHost,
				ForceQuery:  s.ForceQuery,
				RawQuery:    s.RawQuery,
				Fragment:    s.Fragment,
				RawFragment: s.RawFragment,
			}
			b, err := u.MarshalBinary()
			if err != nil {
				return string(b), err
			}
			return string(b), nil
		}),
	)
	return encoder
}
