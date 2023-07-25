package configutil

import "github.com/cortexproject/cortex/pkg/cortex"

type FieldValidationError struct {
	FieldPath string
	Error     error
}

func mkerror(path string, err error) FieldValidationError {
	return FieldValidationError{
		FieldPath: path,
		Error:     err,
	}
}

func ValidateCortexConfig(cfg *cortex.Config) []FieldValidationError {
	panic("todo")
}
