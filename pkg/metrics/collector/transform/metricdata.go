// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Copied from
// https://github.com/open-telemetry/opentelemetry-go/blob/exporters/otlp/otlpmetric/otlpmetricgrpc/v0.39.0/exporters/otlp/otlpmetric/internal/transform/metricdata.go

package transform // import "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/internal/transform"

import (
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	cpb "go.opentelemetry.io/proto/otlp/common/v1"
	mpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	rpb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// ResourceMetrics returns an OTLP ResourceMetrics generated from rm. If rm
// contains invalid ScopeMetrics, an error will be returned along with an OTLP
// ResourceMetrics that contains partial OTLP ScopeMetrics.
func ResourceMetrics(rm *metricdata.ResourceMetrics) (*mpb.ResourceMetrics, error) {
	sms, err := ScopeMetrics(rm.ScopeMetrics)
	return &mpb.ResourceMetrics{
		Resource: &rpb.Resource{
			Attributes: AttrIter(rm.Resource.Iter()),
		},
		ScopeMetrics: sms,
		SchemaUrl:    rm.Resource.SchemaURL(),
	}, err
}

// ScopeMetrics returns a slice of OTLP ScopeMetrics generated from sms. If
// sms contains invalid metric values, an error will be returned along with a
// slice that contains partial OTLP ScopeMetrics.
func ScopeMetrics(sms []metricdata.ScopeMetrics) ([]*mpb.ScopeMetrics, error) {
	errs := &multiErr{datatype: "ScopeMetrics"}
	out := make([]*mpb.ScopeMetrics, 0, len(sms))
	for _, sm := range sms {
		ms, err := Metrics(sm.Metrics)
		if err != nil {
			errs.append(err)
		}

		out = append(out, &mpb.ScopeMetrics{
			Scope: &cpb.InstrumentationScope{
				Name:    sm.Scope.Name,
				Version: sm.Scope.Version,
			},
			Metrics:   ms,
			SchemaUrl: sm.Scope.SchemaURL,
		})
	}
	return out, errs.errOrNil()
}

func ProtoScopeMetrics(sms []*mpb.ScopeMetrics) ([]metricdata.ScopeMetrics, error) {
	errs := &multiErr{datatype: "ScopeMetrics"}
	out := make([]metricdata.ScopeMetrics, 0, len(sms))
	for _, sm := range sms {
		ms, err := ProtoMetrics(sm.Metrics)
		if err != nil {
			errs.append(err)
		}

		out = append(out, metricdata.ScopeMetrics{
			Scope: instrumentation.Scope{
				Name:    sm.Scope.GetName(),
				Version: sm.Scope.GetVersion(),
			},
			Metrics: ms,
		})
	}
	return out, errs.errOrNil()
}

// Metrics returns a slice of OTLP Metric generated from ms. If ms contains
// invalid metric values, an error will be returned along with a slice that
// contains partial OTLP Metrics.
func Metrics(ms []metricdata.Metrics) ([]*mpb.Metric, error) {
	errs := &multiErr{datatype: "Metrics"}
	out := make([]*mpb.Metric, 0, len(ms))
	for _, m := range ms {
		o, err := metric(m)
		if err != nil {
			// Do not include invalid data. Drop the metric, report the error.
			errs.append(errMetric{m: o, err: err})
			continue
		}
		out = append(out, o)
	}
	return out, errs.errOrNil()
}

func ProtoMetrics(ms []*mpb.Metric) ([]metricdata.Metrics, error) {
	errs := &multiErr{datatype: "Metrics"}
	out := make([]metricdata.Metrics, 0, len(ms))
	for _, m := range ms {
		o, err := protoMetric(m)
		if err != nil {
			// Do not include invalid data. Drop the metric, report the error.
			errs.append(errMetric{m: m, err: err})
			continue
		}
		out = append(out, o)
	}
	return out, errs.errOrNil()
}

func metric(m metricdata.Metrics) (*mpb.Metric, error) {
	var err error
	out := &mpb.Metric{
		Name:        m.Name,
		Description: m.Description,
		Unit:        string(m.Unit),
	}
	switch a := m.Data.(type) {
	case metricdata.Gauge[int64]:
		out.Data = Gauge[int64](a)
	case metricdata.Gauge[float64]:
		out.Data = Gauge[float64](a)
	case metricdata.Sum[int64]:
		out.Data, err = Sum[int64](a)
	case metricdata.Sum[float64]:
		out.Data, err = Sum[float64](a)
	case metricdata.Histogram[int64]:
		out.Data, err = Histogram(a)
	case metricdata.Histogram[float64]:
		out.Data, err = Histogram(a)
	default:
		return out, fmt.Errorf("%w: %T", errUnknownAggregation, a)
	}
	return out, err
}

func protoMetric(m *mpb.Metric) (metricdata.Metrics, error) {
	out := metricdata.Metrics{
		Name:        m.GetName(),
		Description: m.GetDescription(),
		Unit:        m.GetUnit(),
	}

	switch d := m.Data.(type) {
	case *mpb.Metric_Gauge:
		pts := d.Gauge.GetDataPoints()
		if len(pts) > 0 {
			switch pts[0].GetValue().(type) {
			case *mpb.NumberDataPoint_AsInt:
				out.Data = ProtoGauge[int64](d)
			case *mpb.NumberDataPoint_AsDouble:
				out.Data = ProtoGauge[float64](d)
			}
		}
	case *mpb.Metric_Sum:
		pts := d.Sum.GetDataPoints()
		if len(pts) > 0 {
			switch pts[0].GetValue().(type) {
			case *mpb.NumberDataPoint_AsInt:
				data, err := ProtoSum[int64](d)
				if err != nil {
					return out, err
				}
				out.Data = data
			case *mpb.NumberDataPoint_AsDouble:
				data, err := ProtoSum[float64](d)
				if err != nil {
					return out, err
				}
				out.Data = data
			}
		}
	case *mpb.Metric_Histogram:
		data, err := ProtoHistogram(d)
		if err != nil {
			return out, err
		}
		out.Data = data
	}

	return out, nil
}

// Gauge returns an OTLP Metric_Gauge generated from g.
func Gauge[N int64 | float64](g metricdata.Gauge[N]) *mpb.Metric_Gauge {
	return &mpb.Metric_Gauge{
		Gauge: &mpb.Gauge{
			DataPoints: DataPoints(g.DataPoints),
		},
	}
}

// Gauge returns a Gauge generated from Metric_Gauge.
func ProtoGauge[N int64 | float64](g *mpb.Metric_Gauge) metricdata.Gauge[N] {
	return metricdata.Gauge[N]{
		DataPoints: ProtoDataPoints[N](g.Gauge.GetDataPoints()),
	}
}

// Sum returns an OTLP Metric_Sum generated from s. An error is returned with
// a partial Metric_Sum if the temporality of s is unknown.
func Sum[N int64 | float64](s metricdata.Sum[N]) (*mpb.Metric_Sum, error) {
	t, err := Temporality(s.Temporality)
	if err != nil {
		return nil, err
	}
	return &mpb.Metric_Sum{
		Sum: &mpb.Sum{
			AggregationTemporality: t,
			IsMonotonic:            s.IsMonotonic,
			DataPoints:             DataPoints(s.DataPoints),
		},
	}, nil
}

func ProtoSum[N int64 | float64](s *mpb.Metric_Sum) (metricdata.Sum[N], error) {
	t, err := ProtoTemporality(s.Sum.GetAggregationTemporality())
	if err != nil {
		return metricdata.Sum[N]{}, err
	}
	return metricdata.Sum[N]{
		Temporality: t,
		IsMonotonic: s.Sum.GetIsMonotonic(),
		DataPoints:  ProtoDataPoints[N](s.Sum.GetDataPoints()),
	}, nil
}

// DataPoints returns a slice of OTLP NumberDataPoint generated from dPts.
func DataPoints[N int64 | float64](dPts []metricdata.DataPoint[N]) []*mpb.NumberDataPoint {
	out := make([]*mpb.NumberDataPoint, 0, len(dPts))
	for _, dPt := range dPts {
		ndp := &mpb.NumberDataPoint{
			Attributes:        AttrIter(dPt.Attributes.Iter()),
			StartTimeUnixNano: uint64(dPt.StartTime.UnixNano()),
			TimeUnixNano:      uint64(dPt.Time.UnixNano()),
		}
		switch v := any(dPt.Value).(type) {
		case int64:
			ndp.Value = &mpb.NumberDataPoint_AsInt{
				AsInt: v,
			}
		case float64:
			ndp.Value = &mpb.NumberDataPoint_AsDouble{
				AsDouble: v,
			}
		}
		out = append(out, ndp)
	}
	return out
}

func ProtoDataPoints[N int64 | float64](dPts []*mpb.NumberDataPoint) []metricdata.DataPoint[N] {
	out := make([]metricdata.DataPoint[N], 0, len(dPts))
	for _, dPt := range dPts {
		kvs := ProtoToKeyValues(dPt.GetAttributes())
		ndp := metricdata.DataPoint[N]{
			Attributes: attribute.NewSet(kvs...),
			StartTime:  time.Unix(0, int64(dPt.GetStartTimeUnixNano())),
			Time:       time.Unix(0, int64(dPt.GetTimeUnixNano())),
		}
		switch dPt.GetValue().(type) {
		case *mpb.NumberDataPoint_AsInt:
			ndp.Value = N(dPt.GetAsInt())
		case *mpb.NumberDataPoint_AsDouble:
			ndp.Value = N(dPt.GetAsDouble())
		}
		out = append(out, ndp)
	}
	return out
}

// Histogram returns an OTLP Metric_Histogram generated from h. An error is
// returned with a partial Metric_Histogram if the temporality of h is
// unknown.
func Histogram[N int64 | float64](h metricdata.Histogram[N]) (*mpb.Metric_Histogram, error) {
	t, err := Temporality(h.Temporality)
	if err != nil {
		return nil, err
	}
	return &mpb.Metric_Histogram{
		Histogram: &mpb.Histogram{
			AggregationTemporality: t,
			DataPoints:             HistogramDataPoints(h.DataPoints),
		},
	}, nil
}

func ProtoHistogram(h *mpb.Metric_Histogram) (metricdata.Histogram[float64], error) {
	t, err := ProtoTemporality(h.Histogram.GetAggregationTemporality())
	if err != nil {
		return metricdata.Histogram[float64]{}, err
	}
	return metricdata.Histogram[float64]{
		Temporality: t,
		DataPoints:  ProtoHistogramDataPoints(h.Histogram.GetDataPoints()),
	}, nil
}

// HistogramDataPoints returns a slice of OTLP HistogramDataPoint generated
// from dPts.
func HistogramDataPoints[N int64 | float64](dPts []metricdata.HistogramDataPoint[N]) []*mpb.HistogramDataPoint {
	out := make([]*mpb.HistogramDataPoint, 0, len(dPts))
	for _, dPt := range dPts {
		sum := float64(dPt.Sum)
		hdp := &mpb.HistogramDataPoint{
			Attributes:        AttrIter(dPt.Attributes.Iter()),
			StartTimeUnixNano: uint64(dPt.StartTime.UnixNano()),
			TimeUnixNano:      uint64(dPt.Time.UnixNano()),
			Count:             dPt.Count,
			Sum:               &sum,
			BucketCounts:      dPt.BucketCounts,
			ExplicitBounds:    dPt.Bounds,
		}
		if v, ok := dPt.Min.Value(); ok {
			vF64 := float64(v)
			hdp.Min = &vF64
		}
		if v, ok := dPt.Max.Value(); ok {
			vF64 := float64(v)
			hdp.Max = &vF64
		}
		out = append(out, hdp)
	}
	return out
}

func ProtoHistogramDataPoints(dPts []*mpb.HistogramDataPoint) []metricdata.HistogramDataPoint[float64] {
	out := make([]metricdata.HistogramDataPoint[float64], 0, len(dPts))
	for _, dPt := range dPts {
		kvs := ProtoToKeyValues(dPt.GetAttributes())
		hdp := metricdata.HistogramDataPoint[float64]{
			Attributes:   attribute.NewSet(kvs...),
			StartTime:    time.Unix(0, int64(dPt.GetStartTimeUnixNano())),
			Time:         time.Unix(0, int64(dPt.GetTimeUnixNano())),
			Count:        dPt.GetCount(),
			BucketCounts: dPt.GetBucketCounts(),
			Bounds:       dPt.GetExplicitBounds(),
			Min:          metricdata.NewExtrema(dPt.GetMin()),
			Max:          metricdata.NewExtrema(dPt.GetMax()),
		}
		out = append(out, hdp)
	}
	return out
}

// Temporality returns an OTLP AggregationTemporality generated from t. If t
// is unknown, an error is returned along with the invalid
// AggregationTemporality_AGGREGATION_TEMPORALITY_UNSPECIFIED.
func Temporality(t metricdata.Temporality) (mpb.AggregationTemporality, error) {
	switch t {
	case metricdata.DeltaTemporality:
		return mpb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, nil
	case metricdata.CumulativeTemporality:
		return mpb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, nil
	default:
		err := fmt.Errorf("%w: %s", errUnknownTemporality, t)
		return mpb.AggregationTemporality_AGGREGATION_TEMPORALITY_UNSPECIFIED, err
	}
}

func ProtoTemporality(t mpb.AggregationTemporality) (metricdata.Temporality, error) {
	switch t {
	case mpb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA:
		return metricdata.DeltaTemporality, nil
	case mpb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE:
		return metricdata.CumulativeTemporality, nil
	default:
		err := fmt.Errorf("%w: %s", errUnknownTemporality, t)
		return 0, err
	}
}
