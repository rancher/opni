package flagutil

import "github.com/spf13/pflag"

type FlagSetter interface {
	FlagSet(...string) *pflag.FlagSet
}
