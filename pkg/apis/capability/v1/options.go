package v1

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rancher/opni/pkg/util"
	"google.golang.org/protobuf/types/known/structpb"
)

type DefaultUninstallOptions struct {
	// If true, will permanently delete all stored data associated with this
	// capability.
	DeleteStoredData bool `json:"deleteStoredData,omitempty"`
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
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(parsed)
	case float64:
		*d = Duration(time.Duration(value))
	default:
		return fmt.Errorf("invalid duration: %v", value)
	}
	return nil
}

func (uo DefaultUninstallOptions) ToStruct() *structpb.Struct {
	m := make(map[string]any)
	data := util.Must(json.Marshal(uo))
	util.Must(json.Unmarshal(data, &m))
	return util.Must(structpb.NewStruct(m))
}

func (uo *DefaultUninstallOptions) LoadFromStruct(s *structpb.Struct) error {
	data, err := json.Marshal(s.AsMap())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, uo)
}
