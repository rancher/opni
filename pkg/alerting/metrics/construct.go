/*
Building promethues / cortex alerting rules
*/
package metrics

import (
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	promql "github.com/prometheus/prometheus/promql/parser"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
)

const NodeFilter = "instance"

type Filter string

func NewFilter() Filter {
	return Filter("")
}

func (f Filter) IsEmpty() bool {
	return string(f) == ""
}

func (f Filter) Or(value string) Filter {
	if f.IsEmpty() {
		return Filter(value)
	}
	return Filter(string(f) + "|" + value)
}

func (f Filter) And(value string) Filter {
	if f.IsEmpty() {
		return Filter(value)
	}
	return Filter(string(f) + "")
}

func (f Filter) Match(key string) Filter {
	if f.IsEmpty() {
		return ""
	}
	return Filter(fmt.Sprintf("%s=~\"%s\"", key, string(f)))
}

func (f Filter) Equals(key string) Filter {
	if f.IsEmpty() {
		return ""
	}
	return Filter(fmt.Sprintf("%s=\"%s\"", key, string(f)))
}

func (f Filter) NotEquals(key string) Filter {
	if f.IsEmpty() {
		return ""
	}
	return Filter(fmt.Sprintf("%s!=~\"%s\"", key, string(f)))
}

type PrometheusFilters struct {
	Filters map[string]Filter
}

func NewPrometheusFilters() *PrometheusFilters {
	return &PrometheusFilters{
		Filters: map[string]Filter{},
	}
}

func (p *PrometheusFilters) AddFilter(key string) {
	if p.Filters == nil {
		p.Filters = map[string]Filter{}
	}
	if _, ok := p.Filters[key]; !ok {
		p.Filters[key] = NewFilter()
	}
}

func (p *PrometheusFilters) Build() string {
	filters := ""
	for _, filter := range p.Filters {
		if filter.IsEmpty() {
			continue
		}
		if filters != "" {
			filters += ","
		}
		filters = filters + string(filter)
	}
	return fmt.Sprintf("{%s}", filters)
}

func (p *PrometheusFilters) Or(key string, value string) {
	p.AddFilter(key)
	p.Filters[key] = p.Filters[key].Or(value)
}

func (p *PrometheusFilters) And(key string, value string) {
	p.AddFilter(key)
	p.Filters[key] = p.Filters[key].And(value)
}

func (p *PrometheusFilters) Match(key string) {
	p.AddFilter(key)
	p.Filters[key] = p.Filters[key].Match(key)
}

func (p *PrometheusFilters) Equals(key string) {
	p.AddFilter(key)
	p.Filters[key] = p.Filters[key].Equals(key)
}

func (p *PrometheusFilters) NotEquals(key string) {
	p.AddFilter(key)
	p.Filters[key] = p.Filters[key].NotEquals(key)
}

const UnlabelledNode = "unidentified_node"
const NodeExporterNodeLabel = "instance"

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
		Labels:      lo.Assign(a.Labels, other.Labels),
		Annotations: lo.Assign(a.Annotations, other.Annotations),
	}
}

func (a *AlertingRule) Or(other *AlertingRule) AlertRuleBuilder {
	return &AlertingRule{
		Alert: "",
		// want Expr to use recording rule names to minimize computations
		Expr:        "(" + a.Alert + ") or (" + other.Alert + ")",
		For:         timeDurationToModelDuration(time.Second * 0),
		Labels:      lo.Assign(a.Labels, other.Labels),
		Annotations: lo.Assign(a.Annotations, other.Annotations),
	}
}

func (a *AlertingRule) IfForSecondsThen(_ *AlertingRule, _ time.Duration) AlertRuleBuilder {
	//TODO : implement
	return nil
}

func (a *AlertingRule) IfNotForSecondsThen(_ *AlertingRule, _ time.Duration) AlertRuleBuilder {
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
	promRule.Annotations = lo.Assign(promRule.Annotations, map[string]string{
		alertingv1.NotificationPropertyOpniUuid: id,
	})
	promRule.Labels = lo.Assign(promRule.Labels, map[string]string{
		alertingv1.NotificationPropertyOpniUuid: id,
	})
	return promRule, nil
}

func WithSloId(sloId, alertType, suffix string) string {
	return fmt.Sprintf("%s-%s%s", sloId, alertType, suffix)
}

// Pretty simple durations for prometheus.
func timeDurationToModelDuration(t time.Duration) model.Duration {
	return model.Duration(t)
}

func PostProcessRuleString(inputString string) string {
	res := strings.ReplaceAll(inputString, "\t", "")
	res = strings.ReplaceAll(res, "\n", "")
	return res
}
