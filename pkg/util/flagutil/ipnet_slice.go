package flagutil

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/spf13/pflag"
)

// Adapted from pflag/ipnet_slice.go

type ipNetSliceValue struct {
	value   *[]string
	changed bool
}

func IPNetSliceValue(val []string, p *[]string) pflag.Value {
	ipnsv := new(ipNetSliceValue)
	ipnsv.value = p
	*ipnsv.value = val
	return ipnsv
}

// Set converts, and assigns, the comma-separated IPNet argument string representation as the []net.IPNet value of this flag.
// If Set is called on a flag that already has a []net.IPNet assigned, the newly converted values will be appended.
func (s *ipNetSliceValue) Set(val string) error {

	// remove all quote characters
	rmQuote := strings.NewReplacer(`"`, "", `'`, "", "`", "")

	// read flag arguments with CSV parser
	ipNetStrSlice, err := readAsCSV(rmQuote.Replace(val))
	if err != nil && err != io.EOF {
		return err
	}

	// parse ip values into slice
	out := make([]string, 0, len(ipNetStrSlice))
	for _, ipNetStr := range ipNetStrSlice {
		_, n, err := net.ParseCIDR(strings.TrimSpace(ipNetStr))
		if err != nil {
			return fmt.Errorf("invalid string being converted to CIDR: %s", ipNetStr)
		}
		out = append(out, n.String())
	}

	if !s.changed {
		*s.value = out
	} else {
		*s.value = append(*s.value, out...)
	}

	s.changed = true

	return nil
}

// Type returns a string that uniquely represents this flag's type.
func (s *ipNetSliceValue) Type() string {
	return "ipNetSlice"
}

// String defines a "native" format for this net.IPNet slice flag value.
func (s *ipNetSliceValue) String() string {
	ipNetStrSlice := make([]string, len(*s.value))
	for i, n := range *s.value {
		ipNetStrSlice[i] = n
	}

	out, _ := writeAsCSV(ipNetStrSlice)
	return "[" + out + "]"
}

func ipNetSliceConv(val string) (interface{}, error) {
	val = strings.Trim(val, "[]")
	// Emtpy string would cause a slice with one (empty) entry
	if len(val) == 0 {
		return []net.IPNet{}, nil
	}
	ss := strings.Split(val, ",")
	out := make([]net.IPNet, len(ss))
	for i, sval := range ss {
		_, n, err := net.ParseCIDR(strings.TrimSpace(sval))
		if err != nil {
			return nil, fmt.Errorf("invalid string being converted to CIDR: %s", sval)
		}
		out[i] = *n
	}
	return out, nil
}

func readAsCSV(val string) ([]string, error) {
	if val == "" {
		return []string{}, nil
	}
	stringReader := strings.NewReader(val)
	csvReader := csv.NewReader(stringReader)
	return csvReader.Read()
}

func writeAsCSV(vals []string) (string, error) {
	b := &bytes.Buffer{}
	w := csv.NewWriter(b)
	err := w.Write(vals)
	if err != nil {
		return "", err
	}
	w.Flush()
	return strings.TrimSuffix(b.String(), "\n"), nil
}
