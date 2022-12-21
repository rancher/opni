package cortex

import (
	"fmt"
	"strings"
	"time"

	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni/pkg/alerting/metrics"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

/*
Contains the struct/function adapters required for opni alerting to
communicate with cortex.
*/

const alertingSuffix = "-opni-alerting"
const defaultAlertingInterval = prommodel.Duration(time.Minute)

// Marshals to the same format a cortex rule group expects
type RuleGroupYAMLv2 struct {
	Name     string             `yaml:"name"`
	Interval prommodel.Duration `yaml:"interval,omitempty"`
	Rules    []rulefmt.Rule     `yaml:"rules"`
}

func CortexRuleIdFromUuid(id string) string {
	return id + alertingSuffix
}

func TimeDurationToPromStr(t time.Duration) string {
	return prommodel.Duration(t).String()
}

func ConstructRecordingRuleName(prefix, typeName string) string {
	return fmt.Sprintf("opni:%s:%s", prefix, typeName)
}

func ConstructIdLabelsForRecordingRule(alertName, alertId string) map[string]string {
	return map[string]string{
		"alert_id":   alertId,
		"alert_name": alertName,
	}
}

func ConstructFiltersFromMap(in map[string]string) string {
	var filters []string
	for k, v := range in {
		filters = append(filters, fmt.Sprintf("%s=\"%s\"", k, v))
	}
	return strings.Join(filters, ",")
}

func NewCortexAlertingRule(
	alertId,
	alertName string,
	info alertingv1.IndexableMetric,
	interval *time.Duration,
	rule metrics.AlertRuleBuilder) (*RuleGroupYAMLv2, error) {
	actualRuleFmt, err := rule.Build(alertId)
	if err != nil {
		return nil, err
	}
	idLabels := ConstructIdLabelsForRecordingRule(alertName, alertId)
	recordingRuleFmt := &rulefmt.Rule{
		Record:      ConstructRecordingRuleName(info.GoldenSignal(), info.AlertType()),
		Expr:        actualRuleFmt.Expr,
		Labels:      idLabels,
		Annotations: map[string]string{},
	}

	actualRuleFmt.Expr = fmt.Sprintf("%s{%s}", recordingRuleFmt.Record, ConstructFiltersFromMap(idLabels))

	var promInterval prommodel.Duration
	if interval == nil {
		promInterval = defaultAlertingInterval
	} else {
		promInterval = prommodel.Duration(*interval)
	}

	return &RuleGroupYAMLv2{
		Name:     CortexRuleIdFromUuid(alertId),
		Interval: promInterval,
		Rules:    []rulefmt.Rule{*actualRuleFmt, *recordingRuleFmt},
	}, nil
}
