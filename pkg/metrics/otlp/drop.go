package otlp

import (
	"regexp"
	"sync"

	metricsotlpv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

type CompOp string

const (
	OpEquals     CompOp = "="
	OpNotEquals  CompOp = "!="
	OpMatches    CompOp = "=~"
	OpNotMatches CompOp = "!~"
)

var (
	PipelineDrop = DropConfig{}
	pipelineMu   sync.Mutex
)

func RegisterDropConfig(cfg DropConfig) {
	pipelineMu.Lock()
	defer pipelineMu.Unlock()
	PipelineDrop = cfg

}

type DropCondition struct {
	Op    CompOp
	Value string
	Re    *regexp.Regexp
}

type DropConfig struct {
	DropCfg []DropCondition `json:"drop"`
}

func (d *DropConfig) Empty() bool {
	return len(d.DropCfg) == 0
}

// Compile optimizes the regex operations specified in the drop config.
func (d *DropConfig) Compile() error {
	for i, op := range d.DropCfg {
		if op.Op == OpMatches || op.Op == OpNotMatches {
			re, err := regexp.Compile(op.Value)
			if err != nil {
				return err
			}
			d.DropCfg[i].Re = re
		}
	}
	return nil
}

func (d *DropConfig) Matches(value string) bool {
	for _, op := range d.DropCfg {
		switch op.Op {
		case OpEquals:
			if value == op.Value {
				return true
			}
		case OpNotEquals:
			if value != op.Value {
				return true
			}
		case OpMatches:
			if op.Re.MatchString(value) {
				return true
			}
		case OpNotMatches:
			if !op.Re.MatchString(value) {
				return true
			}
		}
	}
	return false
}

// create a function that returns hello world

// Assume we never drop resource metrics.
func UnsafeDrop(metrics []*metricsotlpv1.ResourceMetrics, dropCfg DropConfig) []*metricsotlpv1.ResourceMetrics {
	if dropCfg.Empty() {
		return metrics
	}
	res := []*metricsotlpv1.ResourceMetrics{}
	for _, rMetric := range metrics {
		sRes := []*metricsotlpv1.ScopeMetrics{}
		for _, sMetric := range rMetric.ScopeMetrics {
			dropScope := false
			for _, attr := range sMetric.GetScope().GetAttributes() {
				if dropCfg.Matches(attr.Key) {
					dropScope = true
					break
				}
			}
			if dropScope {
				continue
			}
			for _, metric := range sMetric.Metrics {
				switch metric.Data.(type) {
				case *metricsotlpv1.Metric_Gauge:
					for _, d := range metric.GetGauge().GetDataPoints() {
						for _, attr := range d.GetAttributes() {
							if dropCfg.Matches(attr.Key) {
								d = nil
								break
							}
						} // end of data point processing
					}
				case *metricsotlpv1.Metric_Sum:
					for _, d := range metric.GetSum().GetDataPoints() {
						for _, attr := range d.GetAttributes() {
							if dropCfg.Matches(attr.Key) {
								d = nil
								break
							}
						} // end of data point processing
					}
				case *metricsotlpv1.Metric_Histogram:
					for _, d := range metric.GetHistogram().GetDataPoints() {
						for _, attr := range d.GetAttributes() {
							if dropCfg.Matches(attr.Key) {
								d = nil
								break
							}
						} // end of data point processing
					}
				case *metricsotlpv1.Metric_ExponentialHistogram:
					for _, d := range metric.GetExponentialHistogram().GetDataPoints() {
						for _, attr := range d.GetAttributes() {
							if dropCfg.Matches(attr.Key) {
								d = nil
								break
							}
						} // end of data point processing
					}
				case *metricsotlpv1.Metric_Summary:
					for _, d := range metric.GetSummary().GetDataPoints() {
						for _, attr := range d.GetAttributes() {
							if dropCfg.Matches(attr.Key) {
								d = nil
								break
							}
						}
					} // end of data point processing
				}
			} // end of metric processing
			sRes = append(sRes, sMetric)
		} // end scope processing
		rMetric.ScopeMetrics = sRes
	}
	return res
}
