package alerting

import (
	"time"

	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni/pkg/alerting/metrics"
)

const alertingSuffix = "opni-alering"
const defaultAlertingInterval = prommodel.Duration(time.Minute)

type ruleGroupYAMLv2 struct {
	Name     string             `yaml:"name"`
	Interval prommodel.Duration `yaml:"interval,omitempty"`
	Rules    []rulefmt.Rule     `yaml:"rules"`
}

func CortexRuleIdFromUuid(id string) string {
	return id + alertingSuffix
}

func timeDurationToPromStr(t time.Duration) string {
	return prommodel.Duration(t).String()
}

func NewCortexAlertingRule(alertId string, interval *time.Duration, rule metrics.AlertRuleBuilder) (*ruleGroupYAMLv2, error) {
	actualRuleFmt, err := rule.Build(alertId)
	if err != nil {
		return nil, err
	}
	var promInterval prommodel.Duration
	if interval == nil {
		promInterval = defaultAlertingInterval
	} else {
		promInterval = prommodel.Duration(*interval)
	}

	return &ruleGroupYAMLv2{
		Name:     CortexRuleIdFromUuid(alertId),
		Interval: promInterval,
		Rules:    []rulefmt.Rule{*actualRuleFmt},
	}, nil
}
