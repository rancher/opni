package flagutil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

// Adapted from pflag/string_to_int64.go

type stringToFloat64Value struct {
	value   *map[string]float64
	changed bool
}

func StringToFloat64Value(val map[string]float64, p *map[string]float64) pflag.Value {
	ssv := new(stringToFloat64Value)
	ssv.value = p
	*ssv.value = val
	return ssv
}

// Format: a=1,b=2
func (s *stringToFloat64Value) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make(map[string]float64, len(ss))
	for _, pair := range ss {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("%s must be formatted as key=value", pair)
		}
		var err error
		out[kv[0]], err = strconv.ParseFloat(kv[1], 64)
		if err != nil {
			return err
		}
	}
	if !s.changed {
		*s.value = out
	} else {
		for k, v := range out {
			(*s.value)[k] = v
		}
	}
	s.changed = true
	return nil
}

func (s *stringToFloat64Value) Type() string {
	return "stringToInt64"
}

func (s *stringToFloat64Value) String() string {
	var buf bytes.Buffer
	i := 0
	for k, v := range *s.value {
		if i > 0 {
			buf.WriteRune(',')
		}
		buf.WriteString(k)
		buf.WriteRune('=')
		buf.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
		i++
	}
	return "[" + buf.String() + "]"
}
