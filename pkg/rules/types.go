package rules

import (
	"context"

	"github.com/prometheus/prometheus/model/rulefmt"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"
)

type RuleFinder interface {
	FindGroups(ctx context.Context) ([]rulefmt.RuleGroup, error)
}

type UpdateNotifier interface {
	// Returns a channel that will receive a list of rule groups whenever
	// any rules are added, removed, or updated. The channel has a small buffer
	// and will initially contain the latest rule group list.
	//
	// If the context is canceled, the channel will be closed. Additionally, if
	// the channel's buffer is full, any updates will be dropped.
	NotifyC(ctx context.Context) <-chan []rulefmt.RuleGroup
}

func CloneRuleGroupList(list []rulefmt.RuleGroup) []rulefmt.RuleGroup {
	cloned := make([]rulefmt.RuleGroup, len(list))
	for i, g := range list {
		cloned[i] = CloneRuleGroup(g)
	}
	return cloned
}

func CloneRuleGroup(g rulefmt.RuleGroup) rulefmt.RuleGroup {
	cloned := rulefmt.RuleGroup{
		Name:     g.Name,
		Interval: g.Interval,
		Rules:    make([]rulefmt.RuleNode, len(g.Rules)),
	}

	for i, r := range g.Rules {
		cloned.Rules[i] = rulefmt.RuleNode{
			Record: yaml.Node{
				Kind:  r.Record.Kind,
				Tag:   r.Record.Tag,
				Value: r.Record.Value,
			},
			Alert: yaml.Node{
				Kind:  r.Alert.Kind,
				Tag:   r.Alert.Tag,
				Value: r.Alert.Value,
			},
			Expr: yaml.Node{
				Kind:  r.Expr.Kind,
				Tag:   r.Expr.Tag,
				Value: r.Expr.Value,
			},
			For:         r.For,
			Labels:      maps.Clone(r.Labels),
			Annotations: maps.Clone(r.Annotations),
		}
	}
	return cloned
}
