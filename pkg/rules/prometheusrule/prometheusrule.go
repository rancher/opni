package prometheusrule

import (
	"context"
	"fmt"

	"log/slog"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/rules"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PrometheusRuleFinder can find rules defined in PrometheusRule CRDs.
type PrometheusRuleFinder struct {
	PrometheusRuleFinderOptions
	k8sClient client.Client
}

type PrometheusRuleFinderOptions struct {
	logger     *slog.Logger
	namespaces []string
}

type PrometheusRuleFinderOption func(*PrometheusRuleFinderOptions)

func (o *PrometheusRuleFinderOptions) apply(opts ...PrometheusRuleFinderOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithNamespaces(namespaces ...string) PrometheusRuleFinderOption {
	return func(o *PrometheusRuleFinderOptions) {
		o.namespaces = namespaces
	}
}

func WithLogger(lg *slog.Logger) PrometheusRuleFinderOption {
	return func(o *PrometheusRuleFinderOptions) {
		o.logger = lg.WithGroup("rules")
	}
}

func NewPrometheusRuleFinder(k8sClient client.Client, opts ...PrometheusRuleFinderOption) *PrometheusRuleFinder {
	options := PrometheusRuleFinderOptions{
		logger: logger.New().WithGroup("rules"),
	}
	options.apply(opts...)
	return &PrometheusRuleFinder{
		PrometheusRuleFinderOptions: options,
		k8sClient:                   k8sClient,
	}
}

func (f *PrometheusRuleFinder) Find(ctx context.Context) ([]rules.RuleGroup, error) {
	// Find all PrometheusRules
	searchNamespaces := lo.Filter(f.namespaces, func(v string, i int) bool {
		return v != ""
	})
	if len(searchNamespaces) == 0 {
		// No namespaces specified, search all namespaces
		searchNamespaces = append(searchNamespaces, "")
	}

	lg := f.logger.With("namespaces", searchNamespaces)
	var ruleGroups []rules.RuleGroup
	lg.Debug("searching for PrometheusRules")

	for _, namespace := range searchNamespaces {
		groups, err := f.findRulesInNamespace(ctx, namespace)
		if err != nil {
			lg.With(
				logger.Err(err),
				"namespace", namespace,
			).Warn("failed to find PrometheusRules in namespace, skipping")
			continue
		}
		for _, group := range groups {
			// Use Finder[T] alias for rulefmt.RuleGroup
			ruleGroups = append(ruleGroups, rules.RuleGroup(group))
		}
	}

	lg.Debug(fmt.Sprintf("found %d PrometheusRules", len(ruleGroups)))
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
			var interval model.Duration
			var err error
			if group.Interval != nil {
				interval, err = model.ParseDuration(string(*group.Interval))
				if err != nil {
					lg.With(
						"group", group.Name,
					).Warn("skipping rule group: failed to parse group.Interval")
					continue
				}
			}
			ruleNodes := []rulefmt.RuleNode{}
			for _, rule := range group.Rules {
				var ruleFor model.Duration
				if rule.Alert != "" {
					continue
				}
				if rule.For != nil {
					ruleFor, err = model.ParseDuration(string(*rule.For))
					if err != nil {
						lg.With(
							"group", group.Name,
						).Warn("skipping rule: failed to parse rule.For")
						continue
					}
				}
				node := rulefmt.RuleNode{
					For:         ruleFor,
					Labels:      rule.Labels,
					Annotations: rule.Annotations,
				}
				node.Record.SetString(rule.Record)
				node.Expr.SetString(rule.Expr.String())
				if errs := node.Validate(); len(errs) > 0 {
					lg.With(
						"group", group.Name,
						"errs", lo.Map(errs, func(t rulefmt.WrappedError, i int) string {
							return t.Error()
						}),
					).Warn("skipping rule: invalid node")
					continue
				}
				ruleNodes = append(ruleNodes, node)
			}
			if len(ruleNodes) > 0 {
				ruleGroups = append(ruleGroups, rulefmt.RuleGroup{
					Name:     group.Name,
					Interval: interval,
					Rules:    ruleNodes,
				})
			}
		}
	}

	return ruleGroups, nil
}
