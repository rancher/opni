package metrics

import (
	"github.com/prometheus/prometheus/model/rulefmt"
)

type RuleGroupBuilder interface {
	AsRuleGroup() (rulefmt.RuleGroup, error)
}
