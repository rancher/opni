package validation

import (
	"time"

	"github.com/distribution/reference"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/prometheus/common/model"
)

var functions = []cel.EnvOption{
	cel.Function("isValidImageRef",
		cel.MemberOverload("str_valid_oci_image",
			[]*cel.Type{cel.StringType},
			cel.StringType,
			cel.UnaryBinding(func(value ref.Val) ref.Val {
				str, ok := value.Value().(string)
				if !ok {
					return types.UnsupportedRefValConversionErr(value)
				}
				_, err := reference.ParseNormalizedNamed(str)
				if err != nil {
					return types.String(err.Error())
				}
				return types.String("")
			}),
		),
	),
	cel.Function("prometheusDuration",
		cel.Overload("str_prom_model_duration",
			[]*cel.Type{cel.StringType},
			cel.DurationType,
			cel.UnaryBinding(func(value ref.Val) ref.Val {
				str, ok := value.Value().(string)
				if !ok {
					return types.UnsupportedRefValConversionErr(value)
				}
				d, err := model.ParseDuration(str)
				if err != nil {
					return types.WrapErr(err)
				}
				return types.Duration{Duration: time.Duration(d)}
			}),
		),
	),
}
