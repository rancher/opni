package cortexadmin

import (
	"regexp"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rancher/opni/pkg/validation"
)

func (in *PostRuleRequest) Validate() error {
	if in.ClusterId == "" {
		return validation.Error("clusterId is required")
	}
	if in.YamlContent == "" {
		return validation.Error("yamlContent is required")
	}
	return nil
}

func (l *ListRulesRequest) Validate() error {
	if len(l.ClusterId) == 0 {
		return validation.Error("at least one cluster id must be provided")
	}
	for _, cl := range l.ClusterId {
		if cl == "" {
			return validation.Error("clusterId must be set")
		}
	}

	if l.GroupNameRegexp != "" {
		if _, err := regexp.Compile(l.GroupNameRegexp); err != nil {
			return validation.Errorf("invalid regex for group filter %s", l.GroupNameRegexp)
		}
	}

	if l.RuleNameRegexp != "" {
		if _, err := regexp.Compile(l.RuleNameRegexp); err != nil {
			return validation.Errorf("invalid regex for name filter %s", l.RuleNameRegexp)
		}
	}
	if len(l.RuleType) != 0 {
		for _, rt := range l.RuleType {
			if rt == "" {
				return validation.Error("ruleType must be set")
			}
			if rt != string(v1.RuleTypeAlerting) && rt != string(v1.RuleTypeRecording) {
				return validation.Errorf("unsupported ruleType %s, should be one of %s, %s", rt, v1.RuleTypeAlerting, v1.RuleTypeRecording)
			}
		}
	}

	if len(l.HealthFilter) != 0 {
		for _, hf := range l.HealthFilter {
			if hf == "" {
				return validation.Error("healthFilter must be set")
			}
			if hf != v1.RuleHealthGood && hf != v1.RuleHealthBad && hf != v1.RuleHealthUnknown {
				return validation.Errorf(
					"unsupported healthFilter, should be one of : %s", hf, v1.RuleHealthGood, v1.RuleHealthBad, v1.RuleHealthUnknown,
				)
			}
		}
	}
	if len(l.StateFilter) != 0 {
		for _, sf := range l.StateFilter {
			if sf == "" {
				return validation.Error("stateFilter must be set")
			}
			if sf != string(v1.AlertStateFiring) && sf != string(v1.AlertStatePending) && sf != string(v1.AlertStateInactive) {
				return validation.Errorf(
					"unsupported stateFilter %s, should be one of %s, %s, %s", sf, v1.AlertStateFiring, v1.AlertStatePending, v1.AlertStateInactive,
				)
			}
		}
	}
	return nil
}
