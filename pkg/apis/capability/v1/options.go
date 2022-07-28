package v1

import (
	"encoding/json"
	"fmt"
	"time"
)

type DefaultUninstallOptions struct {
	// If true, will permanently delete all stored data associated with this
	// capability.
	DeleteStoredData bool `json:"deleteStoredData,string,omitempty"`
	// Delay the uninstall operation by this amount of time, during which the
	// operation can be canceled without incurring any data loss.
	// If deleteStoredData is false, this option may not have any effect.
	InitialDelay Duration `json:"initialDelay,omitempty"`
}

type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var value any
	if err := json.Unmarshal(b, &value); err != nil {
		return err
	}
	switch value := value.(type) {
	case string:
		if parsed, err := time.ParseDuration(value); err != nil {
			return err
		} else {
			*d = Duration(parsed)
		}
	case float64:
		*d = Duration(time.Duration(value))
	default:
		return fmt.Errorf("invalid duration: %v", value)
	}
	return nil
}
