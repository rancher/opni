package unmarshal

import (
	"encoding/json"
	"fmt"

	"github.com/prometheus/common/model"
)

// Struct for unmarshalling from github.com/prometheus/common/model
type queryResult struct {
	Type   model.ValueType `json:"resultType"`
	Result interface{}     `json:"result"`

	// The decoded value.
	V model.Value
}
type ErrorType string

// struct for unmarshalling from prometheus api responses
type apiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType ErrorType       `json:"errorType"`
	Error     string          `json:"error"`
	Warnings  []string        `json:"warnings,omitempty"`
}

// Unmarshalling for `queryResult`
func (qr *queryResult) UnmarshalJSON(b []byte) error {
	v := struct {
		Type   model.ValueType `json:"resultType"`
		Result json.RawMessage `json:"result"`
	}{}

	err := json.Unmarshal(b, &v)
	if err != nil {
		return err
	}

	switch v.Type {
	case model.ValScalar:
		var sv model.Scalar
		err = json.Unmarshal(v.Result, &sv)
		qr.Type = v.Type
		qr.V = &sv

	case model.ValVector:
		var vv model.Vector
		err = json.Unmarshal(v.Result, &vv)
		qr.Type = v.Type
		qr.V = vv

	case model.ValMatrix:
		var mv model.Matrix
		err = json.Unmarshal(v.Result, &mv)
		qr.Type = v.Type
		qr.V = mv

	default:
		err = fmt.Errorf("unexpected value type %q", v.Type)
	}
	return err
}

func UnmarshalPrometheusResponse(data []byte) (*queryResult, error) {
	var a apiResponse
	var q queryResult

	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(a.Data, &q); err != nil {
		return nil, err
	}
	return &q, nil
}
