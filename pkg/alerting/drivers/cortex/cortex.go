package cortex

import (
	"fmt"
	"strings"
	"time"

	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni/pkg/alerting/metrics"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
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

func ConstructIdLabelsForRecordingRule(alertId string) map[string]string {
	return map[string]string{
		alertingv1.NotificationPropertyOpniUuid: alertId,
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
	opniLabels,
	opniAnnotations map[string]string,
	info alertingv1.IndexableMetric,
	interval *time.Duration,
	rule metrics.AlertRuleBuilder,
) (*RuleGroupYAMLv2, error) {
	idLabels := ConstructIdLabelsForRecordingRule(alertId)
	alertingRule, err := rule.Build(alertId)
	if err != nil {
		return nil, err
	}
	recordingRuleFmt := &rulefmt.Rule{
		Record:      ConstructRecordingRuleName(info.GoldenSignal(), info.AlertType()),
		Expr:        alertingRule.Expr,
		Labels:      idLabels,
		Annotations: map[string]string{},
	}
	// have the alerting rule instead point to the recording rule(s)
	alertingRule.Expr = fmt.Sprintf("%s{%s}", recordingRuleFmt.Record, ConstructFiltersFromMap(idLabels))
	alertingRule.Labels = lo.Assign(alertingRule.Labels, opniLabels)
	alertingRule.Annotations = lo.Assign(alertingRule.Annotations, opniAnnotations)

	var promInterval prommodel.Duration
	if interval == nil {
		promInterval = defaultAlertingInterval
	} else {
		promInterval = prommodel.Duration(*interval)
	}

	return &RuleGroupYAMLv2{
		Name:     CortexRuleIdFromUuid(alertId),
		Interval: promInterval,
		Rules:    []rulefmt.Rule{*alertingRule, *recordingRuleFmt},
	}, nil
}
