package errors

import "errors"

var (
	ErrConfigMissing = errors.New("config value is missing")
)
