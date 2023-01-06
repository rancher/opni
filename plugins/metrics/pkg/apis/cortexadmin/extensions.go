package cortexadmin

import (
	"regexp"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rancher/opni/pkg/validation"
	"golang.org/x/exp/slices"
)

func (in *LoadRuleRequest) Validate() error {
	if in.ClusterId == "" {
		return validation.Error("clusterId is required")
	}
	if len(in.YamlContent) == 0 {
		return validation.Error("yamlContent is required")
	}
	if in.Namespace == "" {
		in.Namespace = "default"
	}
	return nil
}

func (in *GetRuleRequest) Validate() error {
	if in.ClusterId == "" {
		return validation.Error("clusterId is required")
	}
	if in.Namespace == "" {
		in.Namespace = "default"
	}
	if in.GroupName == "" {
		return validation.Error("groupName is required")
	}
	return nil
}

func (in *DeleteRuleRequest) Validate() error {
	if in.ClusterId == "" {
		return validation.Error("clusterId is required")
	}
	if in.Namespace == "" {
		in.Namespace = "default"
	}
	if in.GroupName == "" {
		return validation.Error("groupName is required")
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
	} else {
		l.GroupNameRegexp = ".*"
	}

	if l.RuleNameRegexp != "" {
		if _, err := regexp.Compile(l.RuleNameRegexp); err != nil {
			return validation.Errorf("invalid regex for name filter %s", l.RuleNameRegexp)
		}
	} else {
		l.RuleNameRegexp = ".*"
	}

	if l.NamespaceRegexp != "" {
		if _, err := regexp.Compile(l.NamespaceRegexp); err != nil {
			return validation.Errorf("invalid regex for namespace filter %s", l.NamespaceRegexp)
		}
	} else {
		l.NamespaceRegexp = ".*"
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

func (l *ListRulesRequest) Filter(groups *RuleGroups, clusterId string) *RuleGroups {
	filteredGroup := &RuleGroups{
		Groups: []*RuleGroup{},
	}
	ruleNameRegex := regexp.MustCompile(l.RuleNameRegexp)
	groupNameRegex := regexp.MustCompile(l.GroupNameRegexp)
	namespaceRegex := regexp.MustCompile(l.NamespaceRegexp)

	for _, group := range groups.Groups {
		if !l.MatchesCluster(clusterId) || !l.MatchesRuleGroup(groupNameRegex, group.Name) || !l.MatchesNamespace(namespaceRegex, group.File) {
			continue
		}
		group.ClusterId = clusterId
		matchedRules := []*Rule{}
		for _, rule := range group.Rules {
			if !l.MatchesRuleType(rule.Type) {
				continue
			}
			if rule.Type == string(v1.RuleTypeAlerting) {
				if !l.MatchesRuleState(rule.State) {
					continue
				}
			}
			if !l.MatchesHealth(rule.Health) {
				continue
			}
			if !l.MatchesRule(ruleNameRegex, rule.Name) {
				continue
			}
			matchedRules = append(matchedRules, rule)
		}
		group.Rules = matchedRules
		if len(matchedRules) > 0 {
			filteredGroup.Groups = append(filteredGroup.Groups, group)
		}
	}
	return filteredGroup
}

func xOR(expr1, expr2 bool) bool {
	return expr1 != expr2
}

func (l *ListRulesRequest) MatchesNamespace(namespaceRegex *regexp.Regexp, namespace string) bool {
	if l.All() {
		return true
	}
	return namespaceRegex.MatchString(namespace)
}

func (l *ListRulesRequest) All() bool {
	return l.RequestAll != nil && *l.RequestAll
}

func (l *ListRulesRequest) Invalid() bool {
	return l.ListInvalid != nil && *l.ListInvalid
}

func (l *ListRulesRequest) MatchesCluster(clusterId string) bool {
	if l.All() {
		return true
	}
	// we store each cluster rules in a file with the cluster id
	// and when we search invalid we search specifically those that do not match existing clusters
	return xOR(l.Invalid(), slices.Contains(l.GetClusterId(), clusterId))
}

func (l *ListRulesRequest) MatchesRuleGroup(groupNameExpr *regexp.Regexp, groupName string) bool {
	if l.All() {
		return true
	}
	return groupNameExpr.MatchString(groupName)
}

func (l *ListRulesRequest) MatchesRule(ruleNameExpr *regexp.Regexp, ruleName string) bool {
	if l.All() {
		return true
	}
	return ruleNameExpr.MatchString(ruleName)
}

func (l *ListRulesRequest) MatchesRuleState(state string) bool {
	if l.All() {
		return true
	}
	return (len(l.StateFilter) == 0 || slices.Contains(l.StateFilter, state))
}

func (l *ListRulesRequest) MatchesHealth(health string) bool {
	if l.All() {
		return true
	}
	return len(l.HealthFilter) == 0 || slices.Contains(l.HealthFilter, health)
}

func (l *ListRulesRequest) MatchesRuleType(ruleType string) bool {
	if l.All() {
		return true
	}
	return len(l.RuleType) == 0 || slices.Contains(l.RuleType, ruleType)
}
