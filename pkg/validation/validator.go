package validation

import (
	"github.com/bufbuild/protovalidate-go"
	"github.com/google/cel-go/cel"
)

func NewValidator(options ...protovalidate.ValidatorOption) (*protovalidate.Validator, error) {

	// NewValidator should be used instead of calling protovalidate.New directly
	// so that any custom library functions we define will automatically be
	// available to all validators.
	v, err := protovalidate.New(append(options,
		protovalidate.WithExtendFunc(func(e *cel.Env) []cel.EnvOption {
			return lib(e)
		}),
	)...)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func MustNewValidator(options ...protovalidate.ValidatorOption) *protovalidate.Validator {
	v, err := NewValidator(options...)
	if err != nil {
		panic(err)
	}
	return v
}
