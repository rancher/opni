package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/rules/prometheusrule"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	"gopkg.in/yaml.v3"
)

func (a *Agent) configureRuleFinder() (notifier.Finder[rules.RuleGroup], error) {
	if a.config.Rules != nil {
		if pr := a.config.Rules.Discovery.PrometheusRules; pr != nil {
			client, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
				Kubeconfig: pr.Kubeconfig,
				Scheme:     apis.NewScheme(),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create k8s client: %w", err)
			}
			finder := prometheusrule.NewPrometheusRuleFinder(client,
				prometheusrule.WithLogger(a.logger),
				prometheusrule.WithNamespaces(pr.SearchNamespaces...),
			)
			return finder, nil
		} else if a.config.Rules.Discovery.Filesystem != nil {
			return rules.NewFilesystemRuleFinder(a.config.Rules.Discovery.Filesystem), nil
		}
	}
	return nil, fmt.Errorf("missing configuration")
}

func (a *Agent) streamRuleGroupUpdates(ctx context.Context) (<-chan [][]byte, error) {
	a.logger.Debug("configuring rule discovery")
	finder, err := a.configureRuleFinder()
	if err != nil {
		return nil, fmt.Errorf("failed to configure rule discovery: %w", err)
	}
	a.logger.Debug("rule discovery configured")
	searchInterval := time.Minute * 15
	if interval := a.config.Rules.Discovery.GetInterval(); interval != "" {
		duration, err := time.ParseDuration(interval)
		if err != nil {
			return nil, fmt.Errorf("failed to parse discovery interval: %w", err)
		}
		searchInterval = duration
	}
	notifier := notifier.NewPeriodicUpdateNotifier(ctx, finder, searchInterval)
	a.logger.With(
		"interval", searchInterval.String(),
	).Debug("rule discovery notifier configured")

	notifierC := notifier.NotifyC(ctx)
	a.logger.Debug("starting rule group update notifier")
	groupYamlDocs := make(chan [][]byte, cap(notifierC))
	go func() {
		defer close(groupYamlDocs)
		for {
			ruleGroups, ok := <-notifierC
			if !ok {
				a.logger.Debug("rule discovery channel closed")
				return
			}
			a.logger.Debug("received updated rule groups from discovery")
			go func() {
				groupYamlDocs <- a.marshalRuleGroups(ruleGroups)
			}()
		}
	}()
	return groupYamlDocs, nil
}

func (a *Agent) marshalRuleGroups(ruleGroups []rules.RuleGroup) [][]byte {
	yamlDocs := make([][]byte, 0, len(ruleGroups))
	for _, ruleGroup := range ruleGroups {
		doc, err := yaml.Marshal(ruleGroup)
		if err != nil {
			a.logger.With(
				logger.Err(err),
				"group", ruleGroup.Name,
			).Error("failed to marshal rule group")
			continue
		}
		yamlDocs = append(yamlDocs, doc)
	}
	return yamlDocs
}

func (a *Agent) streamRulesToGateway(actx context.Context) error {
	lg := a.logger
	updateC, err := a.streamRuleGroupUpdates(actx)
	if err != nil {
		return err
	}
	pending := make(chan [][]byte, 1)
	defer close(pending)
	ctx, ca := context.WithCancel(actx)
	defer ca()
	go func() {
		for {
			var docs [][]byte
			select {
			case <-ctx.Done():
				lg.Debug("rule discovery stream closed")
				return
			case docs = <-pending:
			}
		RETRY:
			lg.Debug("sending alert rules to gateway")
			for {
				for _, doc := range docs {
					reqCtx, ca := context.WithTimeout(ctx, time.Second*2)
					var err error
					ok := a.remoteWriteClient.Use(func(rwc remotewrite.RemoteWriteClient) {
						_, err = rwc.SyncRules(reqCtx, &remotewrite.Payload{
							Headers: map[string]string{
								"Content-Type": "application/yaml",
							},
							Contents: doc,
						})
					})
					ca()
					if !ok {
						err = errors.New("not connected")
					}
					if err != nil {
						a.setCondition(condRuleSync, statusFailure, err.Error())
						// retry, unless another update is received from the channel
						lg.With(
							logger.Err(err),
						).Error("failed to send alert rules to gateway (retry in 5 seconds)")
						select {
						case docs = <-pending:
							lg.Debug("updated rules were received during backoff, retrying immediately")
							goto RETRY
						case <-time.After(5 * time.Second):
							goto RETRY
						case <-ctx.Done():
							return
						}
					}
				}
				a.clearCondition(condRuleSync, fmt.Sprintf("successfully sent %d alert rules to gateway", len(docs)))
				break
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			lg.Debug("rule discovery stream closed")
			return nil
		case yamlDocs, ok := <-updateC:
			if !ok {
				lg.Debug("rule discovery stream closed")
				return nil
			}
			lg.Debug("waiting for updated rule documents...")
			pending <- yamlDocs
		}
	}
}
