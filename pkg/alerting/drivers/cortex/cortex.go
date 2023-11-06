package cortex

import (
	"fmt"
	"strings"
	"time"

	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni/pkg/alerting/message"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"
)

/*
Contains the struct/function adapters required for opni alerting to
communicate with cortex.
*/

const (
	MetadataCortexNamespace = "opni.io/cortex-rule-namespace"
	MetadataCortexGroup     = "opni.io/cortex-rule-group"
	MetadataCortexRuleName  = "opni.io/cortex-rule-name"
)

const alertingSuffix = "-opni-alerting"

// this enforces whatever default the remote prometheus instance has
const defaultAlertingInterval = prommodel.Duration(0 * time.Minute)

func RuleIdFromUuid(id string) string {
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
		message.NotificationPropertyOpniUuid: alertId,
	}
}

func ConstructFiltersFromMap(in map[string]string) string {
	var filters []string
	for k, v := range in {
		filters = append(filters, fmt.Sprintf("%s=\"%s\"", k, v))
	}
	return strings.Join(filters, ",")
}

func NewPrometheusAlertingRule(
	alertId,
	_ string,
	opniLabels,
	opniAnnotations map[string]string,
	info alertingv1.IndexableMetric,
	interval *time.Duration,
	rule metrics.AlertRuleBuilder,
) (ruleGroup *rulefmt.RuleGroup, metadata map[string]string, err error) {
	idLabels := ConstructIdLabelsForRecordingRule(alertId)
	alertingRule, err := rule.Build(alertId)
	if err != nil {
		return nil, nil, err
	}
	recordingRuleFmt := &rulefmt.RuleNode{
		Record: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: ConstructRecordingRuleName(info.GoldenSignal(), info.AlertType()),
		},
		Expr: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: alertingRule.Expr.Value,
		},
		Labels:      idLabels,
		Annotations: map[string]string{},
	}
	// have the alerting rule instead point to the recording rule(s)
	alertingRule.Expr.Value = fmt.Sprintf("%s{%s}", recordingRuleFmt.Record.Value, ConstructFiltersFromMap(idLabels))
	alertingRule.Labels = lo.Assign(alertingRule.Labels, opniLabels)
	alertingRule.Annotations = lo.Assign(alertingRule.Annotations, opniAnnotations)

	var promInterval prommodel.Duration
	if interval == nil {
		promInterval = defaultAlertingInterval
	} else {
		promInterval = prommodel.Duration(*interval)
	}

	rg := &rulefmt.RuleGroup{
		Name:     RuleIdFromUuid(alertId),
		Interval: promInterval,
		Rules:    []rulefmt.RuleNode{*alertingRule, *recordingRuleFmt},
	}

	return rg, map[string]string{
		MetadataCortexNamespace: shared.OpniAlertingCortexNamespace,
		MetadataCortexGroup:     rg.Name,
		MetadataCortexRuleName:  alertingRule.Alert.Value,
	}, nil
}
