package otlp

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/samber/lo"
	otlpcommonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsotlpv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

const (
	matchTypeInclude = "include"
	matchTypeExclude = "exclude"
)

type AggregationConfig struct {
	MatchType string
	// does the same but opposite of metricstransform processor aggregation :
	// we only know what we want to aggregate away in generic terms
	// instead of specific set of labels we want to keep
	ExcludeLabelsRe         []*regexp.Regexp
	ExcludeFiltersOneToOne  []func([]*otlpcommonv1.KeyValue) bool
	ExcludeFiltersManyToOne []func(kvs map[string]string) bool
}

type aggGroups struct {
	gauge        map[string][]metricsotlpv1.NumberDataPoint
	sum          map[string][]metricsotlpv1.NumberDataPoint
	histogram    map[string][]metricsotlpv1.HistogramDataPoint
	expHistogram map[string][]metricsotlpv1.ExponentialHistogramDataPoint
}

func reduce(attrs []*otlpcommonv1.KeyValue) map[string]any {
	return lo.Associate(attrs, func(kv *otlpcommonv1.KeyValue) (string, any) {
		return kv.Key, kv.Value
	})
}

// logic like this is good enough for metricstransformprocessor, so it should be for us too
func dataPointHashKey(attrs []*otlpcommonv1.KeyValue, ts uint64, other ...any) string {
	hashParts := []any{reduce(attrs), ts}
	jsonStr, _ := json.Marshal(append(hashParts, other...))
	return string(jsonStr)
}

func groupNumberDataPoints(datapoints []*metricsotlpv1.NumberDataPoint, startTime bool, dest map[string][]metricsotlpv1.NumberDataPoint) {
	var keyHashParts []any
	for i := 0; i < len(datapoints); i++ {
		if startTime {
			keyHashParts = append(keyHashParts, fmt.Sprintf("%d", datapoints[i].StartTimeUnixNano))
		}
		key := dataPointHashKey(datapoints[i].Attributes, datapoints[i].TimeUnixNano, keyHashParts...)
		if _, ok := dest[key]; !ok {
			dest[key] = make([]metricsotlpv1.NumberDataPoint, 0)
		}
		dest[key] = append(dest[key], *datapoints[i])
	}
}

func groupHistogramDatapoints(datapoints []*metricsotlpv1.HistogramDataPoint, startTime bool, dest map[string][]metricsotlpv1.HistogramDataPoint) {
	for i := 0; i < len(datapoints); i++ {
		dp := datapoints[i]
		keyHashParts := make([]any, 0, len(dp.ExplicitBounds)+4)
		for b := 0; b < len(dp.ExplicitBounds); b++ {
			keyHashParts = append(keyHashParts, dp.ExplicitBounds[b])
			if startTime {
				keyHashParts = append(keyHashParts, fmt.Sprintf("%d", dp.StartTimeUnixNano))
			}
			keyHashParts := append(keyHashParts, dp.Min != nil, dp.Max != nil, dp.Flags)
			key := dataPointHashKey(dp.Attributes, dp.TimeUnixNano, keyHashParts...)
			if _, ok := dest[key]; !ok {
				dest[key] = make([]metricsotlpv1.HistogramDataPoint, 0)
			}
			dest[key] = append(dest[key], *datapoints[i])
		}
	}
}

func groupDataPoints(metric metricsotlpv1.Metric, ag aggGroups) aggGroups {
	if metric.GetGauge() != nil {
		if ag.gauge == nil {
			ag.gauge = make(map[string][]metricsotlpv1.NumberDataPoint)
		}
		groupNumberDataPoints(metric.GetSum().DataPoints, false, ag.gauge)

	}
	if metric.GetSum() != nil {
		if ag.sum == nil {
			ag.sum = make(map[string][]metricsotlpv1.NumberDataPoint)
		}
		groupByStartTime := metric.GetSum().GetAggregationTemporality() ==
			metricsotlpv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA //AggregationTemporality(pmetric.AggregationTemporalityDelta)
		groupNumberDataPoints(metric.GetSum().DataPoints, groupByStartTime, ag.sum)
	}
	if metric.GetHistogram() != nil {
		if ag.histogram == nil {
			ag.histogram = make(map[string][]metricsotlpv1.HistogramDataPoint)
		}
		groupHistogramDatapoints(metric.GetHistogram().DataPoints, false, ag.histogram)
	}
	if metric.GetExponentialHistogram() != nil {
		if ag.expHistogram == nil {
			ag.expHistogram = make(map[string][]metricsotlpv1.ExponentialHistogramDataPoint)
		}
		//TODO : nightmare fuel
	}
	return ag
}

func mergeNumberDataPoints() {

}

func mergeDataPoints(metric metricsotlpv1.Metric, aggType string, ag aggGroups) {
	if metric.GetGauge() != nil {
		// mergeNumberDataPoints(ag.gauge, aggType, metric.GetGauge().DataPoints)
	}
	if metric.GetSum() != nil {

	}
	if metric.GetHistogram() != nil {

	}
	if metric.GetExponentialHistogram() != nil {

	}
}

func Aggregate(data []*metricsotlpv1.ResourceMetrics, config AggregationConfig) []*metricsotlpv1.ResourceMetrics {
	// TODO : initial group by
	var ag aggGroups
	for _, resourceMetric := range data {
		for _, scopedMetric := range resourceMetric.ScopeMetrics {
			for i := 0; i < len(scopedMetric.Metrics); i++ {
				ag = groupDataPoints(*scopedMetric.Metrics[i], ag)
			}
		}
	}

	// TODO : merge by
	for _, resource := range data {
		for _, scopedMetrics := range resource.ScopeMetrics {
			for _, metric := range scopedMetrics.Metrics {
				if metric.GetExponentialHistogram() != nil {

				}
				if metric.GetHistogram() != nil {

				}
				if metric.GetSummary() != nil {

				}
				if metric.GetGauge() != nil {

				}
				if metric.GetSum() != nil {

				}
				if metric.GetSummary() != nil {

				}
				// for _, dataPoint := range metric.DataPoints{
				// 	switch dataPoint.(type) {
				// 	case *metricsotlpv1.NumberDataPoint:
				// 	}
			}
		}
	}
	return nil
}
