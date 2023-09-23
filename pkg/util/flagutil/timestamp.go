package flagutil

import (
	"time"

	"github.com/olebedev/when"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type timestamppbValue timestamppb.Timestamp

func TimestamppbValue(val *string, p **timestamppb.Timestamp) pflag.Value {
	if val != nil {
		result, err := when.EN.Parse(*val, time.Now())
		if err != nil {
			panic(err)
		}
		*p = timestamppb.New(result.Time)
	}
	return (*timestamppbValue)(*p)
}

func (d *timestamppbValue) Set(s string) error {
	result, err := when.EN.Parse(s, time.Now())
	if err != nil {
		return err
	}
	*d = *(*timestamppbValue)(timestamppb.New(result.Time))
	return err
}

func (d *timestamppbValue) Type() string {
	return "timestamp"
}

func (d *timestamppbValue) String() string {
	return (*timestamppb.Timestamp)(d).AsTime().String()
}
