package flagutil

import (
	"time"

	"github.com/spf13/pflag"
	"google.golang.org/protobuf/types/known/durationpb"
)

type durationpbValue durationpb.Duration

func DurationpbValue(val time.Duration, p **durationpb.Duration) pflag.Value {
	*p = durationpb.New(val)
	return (*durationpbValue)(*p)
}

func (d *durationpbValue) Set(s string) error {
	v, err := time.ParseDuration(s)
	*d = *(*durationpbValue)(durationpb.New(v))
	return err
}

func (d *durationpbValue) Type() string {
	return "duration"
}

func (d *durationpbValue) String() string { return (*durationpb.Duration)(d).AsDuration().String() }
