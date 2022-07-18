package v1

import "google.golang.org/protobuf/encoding/protojson"

func (uo *UninstallOptions) MarshalJSON() ([]byte, error) {
	return protojson.Marshal(uo)
}

func (uo *UninstallOptions) UnmarshalJSON(data []byte) error {
	return protojson.Unmarshal(data, uo)
}
