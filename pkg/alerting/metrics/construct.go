/*
Building promethues / cortex alerting rules
*/
package metrics

import (
	"fmt"
	promql "github.com/cortexproject/cortex/pkg/configs/legacy_promql"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni/pkg/alerting/shared"
	"time"
)

// AlertingRule
//
// Wrapper around rulefmt.Rule that implements
// the AlertingRuleBuilder interface.
type AlertingRule struct {
	Alert       string
	Expr        string
	For         model.Duration
	Labels      map[string]string
	Annotations map[string]string
}

func (a *AlertingRule) Validate() error {
	if a.Alert == "" {
		return fmt.Errorf("alert name is required")
	}
	_, err := promql.ParseExpr(a.Expr)
	return err
}

type AlertRuleBuilder interface {
	And(rule *AlertingRule) AlertRuleBuilder
	Or(rule *AlertingRule) AlertRuleBuilder
	IfForSecondsThen(*AlertingRule, time.Duration) AlertRuleBuilder
	IfNotForSecondsThen(*AlertingRule, time.Duration) AlertRuleBuilder
	Build(id string) (*rulefmt.Rule, error)
}

func (a *AlertingRule) AsRuleFmt() *rulefmt.Rule {
	return &rulefmt.Rule{
		Alert:       a.Alert,
		Expr:        a.Expr,
		For:         a.For,
		Labels:      a.Labels,
		Annotations: a.Annotations,
	}
}

func (a *AlertingRule) And(other *AlertingRule) AlertRuleBuilder {
	return &AlertingRule{
		Alert: "",
		// want Expr to use recording rule names to minimize computations
		Expr:        "(" + a.Alert + ") and (" + other.Alert + ")",
		For:         timeDurationToModelDuration(time.Second * 0),
		Labels:      MergeLabels(a.Labels, other.Labels),
		Annotations: MergeLabels(a.Annotations, other.Annotations),
	}
}

func (a *AlertingRule) Or(other *AlertingRule) AlertRuleBuilder {
	return &AlertingRule{
		Alert: "",
		// want Expr to use recording rule names to minimize computations
		Expr:        "(" + a.Alert + ") or (" + other.Alert + ")",
		For:         timeDurationToModelDuration(time.Second * 0),
		Labels:      MergeLabels(a.Labels, other.Labels),
		Annotations: MergeLabels(a.Annotations, other.Annotations),
	}
}

func (a *AlertingRule) IfForSecondsThen(other *AlertingRule, seconds time.Duration) AlertRuleBuilder {
	//TODO : implement
	return nil
}

func (a *AlertingRule) IfNotForSecondsThen(other *AlertingRule, seconds time.Duration) AlertRuleBuilder {
	//TODO: implement
	return nil
}

func (a *AlertingRule) Build(id string) (*rulefmt.Rule, error) {
	promRule := a.AsRuleFmt()
	promRule.Alert = id
	_, err := promql.ParseExpr(promRule.Expr)
	if err != nil {
		return nil, fmt.Errorf("constructed rule : %s is not a valid prometheus rule %v", promRule.Expr, err)
	}
	promRule.Annotations = MergeLabels(promRule.Annotations, map[string]string{
		shared.BackendConditionIdLabel: id,
	})
	promRule.Labels = MergeLabels(promRule.Labels, map[string]string{
		shared.BackendConditionIdLabel: id,
	})
	return promRule, nil
}

// Pretty simple durations for prometheus.
func timeDurationToModelDuration(t time.Duration) model.Duration {
	return model.Duration(t)
}

func MergeLabels(ms ...map[string]string) map[string]string {
	res := map[string]string{}
	for _, m := range ms {
		for k, v := range m {
			res[k] = v
		}
	}
	return res
}
