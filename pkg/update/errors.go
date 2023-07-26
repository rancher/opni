package update

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidURN         = errors.New("invalid URN")
	ErrInvalidType        = errors.New("type must be one of plugin or agent")
	ErrMultipleStrategies = errors.New("multiple strategies found in manifest")
	ErrMultipleTypes      = errors.New("multiple types found in manifest")
	ErrNoEntries          = errors.New("no entries found in manifest")
	ErrInvalidConfig      = errors.New("invalid config")
)

func ErrInvalidNamespace(ns string) error {
	return fmt.Errorf("invalid namespace - %s: %w", ns, ErrInvalidURN)
}
