package rules

import (
	"context"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FooOptions struct {
	bar string
}

type FooOption func(*FooOptions)

func (o *FooOptions) Apply(opts ...FooOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithBar(bar string) FooOption {
	return func(o *FooOptions) {
		o.bar = bar
	}
}

// PrometheusRuleFinder can find rules defined in PrometheusRule CRDs.
type PrometheusRuleFinder struct {
	PrometheusRuleFinderOptions
	k8sClient client.Client
}

type PrometheusRuleFinderOptions struct {
	logger     *zap.SugaredLogger
	namespaces []string
}

type PrometheusRuleFinderOption func(*PrometheusRuleFinderOptions)

func (o *PrometheusRuleFinderOptions) Apply(opts ...PrometheusRuleFinderOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithNamespaces(namespaces ...string) PrometheusRuleFinderOption {
	return func(o *PrometheusRuleFinderOptions) {
		o.namespaces = namespaces
	}
}

func WithLogger(lg *zap.SugaredLogger) PrometheusRuleFinderOption {
	return func(o *PrometheusRuleFinderOptions) {
		o.logger = lg.Named("rules")
	}
}

func NewPrometheusRuleFinder(k8sClient client.Client, opts ...PrometheusRuleFinderOption) RuleFinder {
	options := PrometheusRuleFinderOptions{
		logger: logger.New().Named("rules"),
	}
	options.Apply(opts...)
	return &PrometheusRuleFinder{
		PrometheusRuleFinderOptions: options,
		k8sClient:                   k8sClient,
	}
}

func (f *PrometheusRuleFinder) FindGroups(ctx context.Context) ([]rulefmt.RuleGroup, error) {
	lg := f.logger.With("namespaces", f.namespaces)
	var ruleGroups []rulefmt.RuleGroup
	lg.Debug("searching for PrometheusRules")

	// Find all PrometheusRules
	searchNamespaces := []string{}
	switch {
	case len(f.namespaces) == 0:
		// No namespaces specified, search all namespaces
		f.namespaces = append(f.namespaces, "")
	case len(f.namespaces) > 1:
		// If multiple namespaces are specified, filter out empty strings, which
		// would otherwise match all namespaces.
		for _, ns := range f.namespaces {
			if ns != "" {
				searchNamespaces = append(searchNamespaces, ns)
			}
		}
	}
	for _, namespace := range f.namespaces {
		groups, err := f.findRulesInNamespace(ctx, namespace)
		if err != nil {
			lg.With(
				"namespace", namespace,
			).Warn("failed to find PrometheusRules in namespace, skipping")
			continue
		}
		ruleGroups = append(ruleGroups, groups...)
	}

	lg.Debugf("found %d PrometheusRules", len(ruleGroups))
	return ruleGroups, nil
}

func (f *PrometheusRuleFinder) findRulesInNamespace(
	ctx context.Context,
	namespace string,
) ([]rulefmt.RuleGroup, error) {
	lg := f.logger

	promRules := &monitoringv1.PrometheusRuleList{}
	if err := f.k8sClient.List(ctx, promRules, client.InNamespace(namespace)); err != nil {
		return nil, err
	}

	// Convert PrometheusRules to rulefmt.RuleGroup
	var ruleGroups []rulefmt.RuleGroup
	for _, promRule := range promRules.Items {
		for _, group := range promRule.Spec.Groups {
			interval, err := model.ParseDuration(group.Interval)
			if err != nil {
				lg.With(
					"group", group.Name,
				).Warn("skipping rule group: failed to parse group.Interval")
			}
			rules := []rulefmt.RuleNode{}
			for _, rule := range group.Rules {
				ruleFor, err := model.ParseDuration(rule.For)
				if err != nil {
					lg.With(
						"group", group.Name,
					).Warn("skipping rule: failed to parse rule.For")
				}
				rules = append(rules, rulefmt.RuleNode{
					Record:      yaml.Node{Value: rule.Record},
					Alert:       yaml.Node{Value: rule.Alert},
					Expr:        yaml.Node{Value: rule.Expr.String()},
					For:         ruleFor,
					Labels:      rule.Labels,
					Annotations: rule.Annotations,
				})
			}
			ruleGroups = append(ruleGroups, rulefmt.RuleGroup{
				Name:     group.Name,
				Interval: interval,
				Rules:    rules,
			})
		}
	}

	return ruleGroups, nil
}
