package kubernetes

import (
	"fmt"
)

var (
	ErrInvalidAgentList  = fmt.Errorf("agent deployments found not exactly 1")
	ErrOldDigestMismatch = fmt.Errorf("current digest does not match patch spec")
	ErrInvalidUpdateType = fmt.Errorf("invalid update type")
)

func ErrComponentUpdate(component ComponentType, err error) error {
	return fmt.Errorf("error updating %s: %w", component, err)
}

func ErrUnhandledUpdateType(updateType string) error {
	return fmt.Errorf("no handler for update type %s: %w", updateType, ErrInvalidUpdateType)
}
