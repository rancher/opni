package config

import (
	"reflect"
	"strings"

	"dagger.io/dagger"
	"github.com/spf13/pflag"
)

func BuildFlagSet(t reflect.Type, prefix ...string) *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tagValue := field.Tag.Get("koanf")
		if tagValue == "" {
			continue
		}
		flagName := strings.Join(append(prefix, tagValue), ".")
		if field.Type == reflect.TypeOf(&dagger.Secret{}) {
			continue
		}
		switch kind := field.Type.Kind(); kind {
		case reflect.String:
			fs.String(flagName, "", tagValue)
		case reflect.Slice:
			switch elemKind := field.Type.Elem().Kind(); elemKind {
			case reflect.String:
				fs.StringSlice(flagName, nil, tagValue)
			default:
				panic("unimplemented: []" + elemKind.String())
			}
		case reflect.Bool:
			fs.Bool(flagName, false, tagValue)
		case reflect.Struct:
			fs.AddFlagSet(BuildFlagSet(field.Type, append(prefix, tagValue)...))
		default:
			panic("unimplemented: " + kind.String())
		}
	}

	fs.VisitAll(func(f *pflag.Flag) {
		fs.MarkHidden(f.Name)
	})
	return fs
}
