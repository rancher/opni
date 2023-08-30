package flagutil

import (
	"github.com/spf13/pflag"
)

func LoadDefaults[T interface {
	FlagSet(...string) *pflag.FlagSet
}](obj T) {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs.AddFlagSet(obj.FlagSet())
	fs.Parse(nil)
}

// Sets DefValue and calls Value.Set for a flag. Must only be used after calling
// AddFlagSet and before calling Parse.
func SetDefValue(fs *pflag.FlagSet, name, value string) {
	f := fs.Lookup(name)
	if f == nil {
		panic("flag not found")
	}
	f.DefValue = value
	f.Value.Set(value)
}
