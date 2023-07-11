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
