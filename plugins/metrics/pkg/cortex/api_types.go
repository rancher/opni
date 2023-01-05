package cortex

import (
	"regexp"

	"golang.org/x/exp/slices"
)

func xOR(expr1, expr2 bool) bool {
	return expr1 != expr2
}

type listRuleFilter struct {
	clusters     []string
	healthFilter []string
	stateFilter  []string
	ruleType     []string
	ruleFilter   *regexp.Regexp
	groupFilter  *regexp.Regexp
	onlyInvalid  bool
	all          bool
}

func NewListRuleFilter(
	clusters,
	ruleType,
	healthFilters,
	stateFilters []string,
	ruleNameRegexp,
	groupNameRegexp string,
	invalid *bool,
	all *bool,
) (*listRuleFilter, error) {
	var ruleFilter *regexp.Regexp
	if ruleNameRegexp != "" {
		r, err := regexp.Compile(ruleNameRegexp)
		if err != nil {
			return nil, err
		}
		ruleFilter = r
	} else {
		ruleFilter = regexp.MustCompile(".*")
	}

	var groupFilter *regexp.Regexp
	if groupNameRegexp != "" {
		r, err := regexp.Compile(groupNameRegexp)
		if err != nil {
			return nil, err
		}
		groupFilter = r
	} else {
		groupFilter = regexp.MustCompile(".*")
	}

	var searchInvalid bool
	if invalid != nil {
		searchInvalid = *invalid
	} else {
		searchInvalid = false
	}

	var returnAll bool
	if all != nil {
		returnAll = *all
	} else {
		returnAll = false
	}
	return &listRuleFilter{
		clusters:     clusters,
		ruleType:     ruleType,
		healthFilter: healthFilters,
		stateFilter:  stateFilters,
		ruleFilter:   ruleFilter,
		groupFilter:  groupFilter,
		onlyInvalid:  searchInvalid,
		all:          returnAll,
	}, nil
}

func (f *listRuleFilter) MatchesCluster(clusterId string) bool {
	if f.all {
		return true
	}
	// we store each cluster rules in a file with the cluster id
	// and when we search invalid we search specifically those that do not match existing clusters
	return xOR(f.onlyInvalid, slices.Contains(f.clusters, clusterId))

}

func (f *listRuleFilter) MatchesRuleGroup(groupName string) bool {
	if f.all {
		return true
	}
	return f.groupFilter.MatchString(groupName)
}

func (f *listRuleFilter) MatchesRule(ruleName string) bool {
	if f.all {
		return true
	}
	return f.ruleFilter.MatchString(ruleName)
}

func (f *listRuleFilter) MatchesRuleState(state string) bool {
	if f.all {
		return true
	}
	return (len(f.stateFilter) == 0 || slices.Contains(f.stateFilter, state))
}

func (f *listRuleFilter) MatchesHealth(health string) bool {
	if f.all {
		return true
	}
	return len(f.healthFilter) == 0 || slices.Contains(f.healthFilter, health)
}

func (f *listRuleFilter) MatchesRuleType(ruleType string) bool {
	if f.all {
		return true
	}
	return len(f.ruleType) == 0 || slices.Contains(f.ruleType, ruleType)
}
