package flagutil

import (
	"net"
	"strings"

	"github.com/spf13/pflag"
)

// IPNet adapts net.IPNet for use as a flag.
type ipNetValue string

func (i ipNetValue) String() string {
	return string(i)
}

func (i *ipNetValue) Set(value string) error {
	_, n, err := net.ParseCIDR(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	*i = ipNetValue(n.String())
	return nil
}

func (*ipNetValue) Type() string {
	return "ipNet"
}

func IPNetValue(val string, p *string) pflag.Value {
	*p = val
	return (*ipNetValue)(p)
}
