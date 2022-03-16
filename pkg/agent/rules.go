package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni-monitoring/pkg/clients"
	"github.com/rancher/opni-monitoring/pkg/rules"
	"github.com/rancher/opni-monitoring/pkg/util"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

func (a *Agent) configureRuleFinder() (rules.RuleFinder, error) {
	if a.Rules != nil {
		if pr := a.Rules.Discovery.PrometheusRules; pr != nil {
			client, err := util.NewK8sClient(util.ClientOptions{
				Kubeconfig: pr.Kubeconfig,
			})
			if err != nil {
				return nil, err
			}
			finder := rules.NewPrometheusRuleFinder(client,
				rules.WithLogger(a.logger),
				rules.WithNamespaces(pr.SearchNamespaces...),
			)
			return finder, nil
		}
	}
	return nil, fmt.Errorf("missing configuration")
}

func (a *Agent) streamRuleGroupUpdates(ctx context.Context) (<-chan [][]byte, error) {
	finder, err := a.configureRuleFinder()
	if err != nil {
		return nil, fmt.Errorf("failed to configure rule discovery: %w", err)
	}
	searchInterval := time.Minute * 15
	if interval := a.Rules.Discovery.Interval; interval != "" {
		duration, err := time.ParseDuration(interval)
		if err != nil {
			return nil, fmt.Errorf("failed to parse discovery interval: %w", err)
		}
		searchInterval = duration
	}
	notifier := rules.NewPeriodicUpdateNotifier(ctx, finder, searchInterval)

	notifierC := notifier.NotifyC(ctx)
	groupYamlDocs := make(chan [][]byte, cap(notifierC))
	go func() {
		defer close(groupYamlDocs)
		for {
			ruleGroups, ok := <-notifierC
			if !ok {
				a.logger.Debug("rule discovery channel closed")
				return
			}
			go func() {
				groupYamlDocs <- a.marshalRuleGroups(ruleGroups)
			}()
		}
	}()
	return groupYamlDocs, nil
}

func (a *Agent) marshalRuleGroups(ruleGroups []rulefmt.RuleGroup) [][]byte {
	var yamlDocs [][]byte
	for _, ruleGroup := range ruleGroups {
		doc, err := yaml.Marshal(ruleGroup)
		if err != nil {
			a.logger.With(
				zap.Error(err),
				zap.String("group", ruleGroup.Name),
			).Error("failed to marshal rule group")
			continue
		}
		yamlDocs = append(yamlDocs, doc)
	}
	return yamlDocs
}

func (a *Agent) streamRulesToGateway(ctx context.Context, client clients.GatewayHTTPClient) error {
	updateC, err := a.streamRuleGroupUpdates(ctx)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case yamlDocs, ok := <-updateC:
			if !ok {
				return nil
			}
			for _, yamlDoc := range yamlDocs {
				code, _, err := client.Post(ctx, "/api/agent/rules").
					Header("Content-Type", "application/yaml").
					Body(yamlDoc).
					Send()
				if err != nil || code != 200 {

					// retry, unless another update is received from the channel
				}
			}
		}
	}
}
