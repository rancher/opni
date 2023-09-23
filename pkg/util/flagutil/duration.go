package flagutil

import (
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/types/known/durationpb"
)

type durationpbValue durationpb.Duration

func DurationpbValue(val *time.Duration, p **durationpb.Duration) pflag.Value {
	if val != nil {
		*p = durationpb.New(*val)
	}
	return (*durationpbValue)(*p)
}

func (d *durationpbValue) Set(s string) error {
	v, err := ParseDurationWithExtendedUnits(s)
	*d = *(*durationpbValue)(durationpb.New(v))
	return err
}

func (d *durationpbValue) Type() string {
	return "duration"
}

func (d *durationpbValue) String() string {
	return (*durationpb.Duration)(d).AsDuration().String()
}

func ParseDurationWithExtendedUnits(s string) (time.Duration, error) {
	// try parsing with standard units
	v, err := time.ParseDuration(s)
	if err != nil {
		if strings.Contains(err.Error(), "unknown unit") {
			// prometheus duration accepts "w" and "y" units, for some reason
			p, err := model.ParseDuration(s)
			if err != nil {
				return 0, err
			}
			v = time.Duration(p)
		}
	}
	return v, nil
}
