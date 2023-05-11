package flagutil

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Adapted from pflag/duration_slice.go

type durationSliceValue struct {
	value   *[]*durationpb.Duration
	changed bool
}

func DurationpbSliceValue(val []time.Duration, p *[]*durationpb.Duration) pflag.Value {
	dsv := new(durationSliceValue)
	dsv.value = p
	converted := make([]*durationpb.Duration, len(val))
	for i, d := range val {
		converted[i] = durationpb.New(d)
	}
	*dsv.value = converted
	return dsv
}

func (s *durationSliceValue) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make([]*durationpb.Duration, len(ss))
	for i, d := range ss {
		var err error
		td, err := time.ParseDuration(d)
		if err != nil {
			return err
		}
		out[i] = durationpb.New(td)
	}
	if !s.changed {
		*s.value = out
	} else {
		*s.value = append(*s.value, out...)
	}
	s.changed = true
	return nil
}

func (s *durationSliceValue) Type() string {
	return "durationSlice"
}

func (s *durationSliceValue) String() string {
	out := make([]string, len(*s.value))
	for i, d := range *s.value {
		out[i] = fmt.Sprintf("%s", d.AsDuration())
	}
	return "[" + strings.Join(out, ",") + "]"
}

func (s *durationSliceValue) fromString(val string) (*durationpb.Duration, error) {
	d, err := time.ParseDuration(val)
	if err != nil {
		return nil, err
	}
	return durationpb.New(d), nil
}

func (s *durationSliceValue) toString(val *durationpb.Duration) string {
	return fmt.Sprintf("%s", val)
}

func (s *durationSliceValue) Append(val string) error {
	i, err := s.fromString(val)
	if err != nil {
		return err
	}
	*s.value = append(*s.value, i)
	return nil
}

func (s *durationSliceValue) Replace(val []string) error {
	out := make([]*durationpb.Duration, len(val))
	for i, d := range val {
		var err error
		td, err := s.fromString(d)
		if err != nil {
			return err
		}
		out[i] = td
	}
	*s.value = out
	return nil
}

func (s *durationSliceValue) GetSlice() []string {
	out := make([]string, len(*s.value))
	for i, d := range *s.value {
		out[i] = s.toString(d)
	}
	return out
}
