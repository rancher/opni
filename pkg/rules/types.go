package rules

import (
	"github.com/prometheus/prometheus/model/rulefmt"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"
)

// Alias so we can implement the Finder[T] interface
// on Prometheus' rulefmt.RuleGroup
type RuleGroup rulefmt.RuleGroup
type RuleGroupList []RuleGroup

func (r RuleGroup) Clone() RuleGroup {
	cloned := RuleGroup{
		Name:     r.Name,
		Interval: r.Interval,
		Rules:    make([]rulefmt.RuleNode, len(r.Rules)),
	}

	for i, r := range r.Rules {
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

// func CloneRuleGroupList(list []rulefmt.RuleGroup) []rulefmt.RuleGroup {
// 	cloned := make([]rulefmt.RuleGroup, len(list))
// 	for i, g := range list {
// 		cloned[i] = CloneRuleGroup(g)
// 	}
// 	return cloned
// }

// func CloneRuleGroup(g rulefmt.RuleGroup) rulefmt.RuleGroup {
// 	cloned := rulefmt.RuleGroup{
// 		Name:     g.Name,
// 		Interval: g.Interval,
// 		Rules:    make([]rulefmt.RuleNode, len(g.Rules)),
// 	}

// 	for i, r := range g.Rules {
// 		cloned.Rules[i] = rulefmt.RuleNode{
// 			Record: yaml.Node{
// 				Kind:  r.Record.Kind,
// 				Tag:   r.Record.Tag,
// 				Value: r.Record.Value,
// 			},
// 			Alert: yaml.Node{
// 				Kind:  r.Alert.Kind,
// 				Tag:   r.Alert.Tag,
// 				Value: r.Alert.Value,
// 			},
// 			Expr: yaml.Node{
// 				Kind:  r.Expr.Kind,
// 				Tag:   r.Expr.Tag,
// 				Value: r.Expr.Value,
// 			},
// 			For:         r.For,
// 			Labels:      maps.Clone(r.Labels),
// 			Annotations: maps.Clone(r.Annotations),
// 		}
// 	}
// 	return cloned
// }
