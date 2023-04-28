package flagutil

import (
	"strconv"

	"github.com/spf13/pflag"
)

type boolPtrFlag interface {
	pflag.Value
	IsBoolFlag() bool
}

type boolPtrValue struct {
	p **bool
}

func BoolPtrValue(p **bool) pflag.Value {
	return &boolPtrValue{p}
}

func (b *boolPtrValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	*b.p = &v
	return err
}

func (b *boolPtrValue) Type() string {
	return "bool"
}

func (b *boolPtrValue) String() string {
	if *b.p == nil {
		return "nil"
	}
	return strconv.FormatBool(**b.p)
}

func (b *boolPtrValue) IsBoolFlag() bool { return true }
