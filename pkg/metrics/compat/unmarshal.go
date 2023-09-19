package compat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/prometheus/common/model"
	"go.uber.org/zap"
)

// Struct for unmarshalling from github.com/prometheus/common/model
type QueryResult struct {
	Type   model.ValueType `json:"resultType"`
	Result interface{}     `json:"result"`

	// The decoded value.
	V model.Value
}
type ErrorType string

// struct for unmarshalling from prometheus api responses
type ApiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType ErrorType       `json:"errorType"`
	Error     string          `json:"error"`
	Warnings  []string        `json:"warnings,omitempty"`
}

// Unmarshalling for `queryResult`
func (qr *QueryResult) UnmarshalJSON(b []byte) error {
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

func (qr *QueryResult) GetVector() (*model.Vector, error) {
	switch qr.V.Type() {
	case model.ValVector:
		v := qr.V.(model.Vector)
		return &v, nil
	default:
		return nil, fmt.Errorf("cannot unmarshal prometheus response into vector type")
	}
}

func (qr *QueryResult) GetMatrix() (*model.Matrix, error) {
	switch qr.V.Type() {
	case model.ValMatrix:
		v := qr.V.(model.Matrix)
		return &v, nil
	default:
		return nil, fmt.Errorf("cannot unmarshal prometheus response into matrix type")
	}
}

func (qr *QueryResult) GetScalar() (*model.Scalar, error) {
	switch qr.V.Type() {
	case model.ValScalar:
		v := *qr.V.(*model.Scalar)
		return &v, nil
	default:
		return nil, fmt.Errorf("cannot unmarshal prometheus response into scalar type")
	}
}

type Sample struct {
	Value     float64
	Timestamp int64
}

func (qr *QueryResult) LinearSamples() []Sample {
	res := []Sample{}
	switch qr.V.Type() {
	case model.ValVector:
		v := qr.V.(model.Vector)
		for _, sample := range v {
			res = append(res, Sample{
				Value:     float64(sample.Value),
				Timestamp: int64(sample.Timestamp),
			})
		}
	case model.ValScalar:
		v := qr.V.(*model.Scalar)
		res = append(res, Sample{
			Value:     float64(v.Value),
			Timestamp: int64(v.Timestamp),
		})
	case model.ValMatrix:
		sampleStream := qr.V.(model.Matrix)
		for _, sample := range sampleStream {
			for _, s := range sample.Values {
				res = append(res, Sample{
					Value:     float64(s.Value),
					Timestamp: int64(s.Timestamp),
				})
			}
		}
	default:
		panic("bug: unreachable code")
	}

	slices.SortFunc(res, func(i, j Sample) int {
		return int(i.Timestamp - j.Timestamp)
	})
	return res
}

func UnmarshalPrometheusResponse(data []byte) (*QueryResult, error) {
	var a ApiResponse
	var q QueryResult

	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(a.Data, &q); err != nil {
		return nil, err
	}
	return &q, nil
}

// https://github.com/prometheus/prometheus/blob/main/web/api/v1/api.go
type status string

const (
	statusSuccess status = "success"
	statusError   status = "error"
)

// https://github.com/prometheus/prometheus/blob/main/web/api/v1/api.go
type errorType string

const (
	errorNone        errorType = ""
	errorTimeout     errorType = "timeout"
	errorCanceled    errorType = "canceled"
	errorExec        errorType = "execution"
	errorBadData     errorType = "bad_data"
	errorInternal    errorType = "internal"
	errorUnavailable errorType = "unavailable"
	errorNotFound    errorType = "not_found"
)

// Generic struct for unmarshalling prometheus http api responses
// https://github.com/prometheus/prometheus/blob/bcd548c88b06543c8eeb19e68bef4adefb7b95fb/web/api/v1/api.go#L140
type Response struct {
	Status    status      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType errorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
}

func UnmarshallPrometheusWebResponse(resp *http.Response, _ *zap.SugaredLogger) (*Response, error) {
	var val *Response
	err := json.NewDecoder(resp.Body).Decode(&val)
	if err != nil {
		return nil, err
	}
	if val.Status != statusSuccess {
		return nil, fmt.Errorf("well formed prometheus request failed internally with: %s", val.Error)
	}
	return val, nil
}
