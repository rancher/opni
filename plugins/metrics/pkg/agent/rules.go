package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type RuleStreamer struct {
	logger              *zap.SugaredLogger
	remoteWriteClientMu sync.Mutex
	remoteWriteClient   remotewrite.RemoteWriteClient
	conditions          ConditionTracker
}

func NewRuleStreamer(ct ConditionTracker, lg *zap.SugaredLogger) *RuleStreamer {
	return &RuleStreamer{
		logger:     lg,
		conditions: ct,
	}
}

func (s *RuleStreamer) SetRemoteWriteClient(client remotewrite.RemoteWriteClient) {
	s.remoteWriteClientMu.Lock()
	defer s.remoteWriteClientMu.Unlock()
	s.remoteWriteClient = client
}

func (s *RuleStreamer) Run(ctx context.Context, config *v1beta1.RulesSpec) error {
	lg := s.logger
	updateC, err := s.streamRuleGroupUpdates(ctx, config)
	if err != nil {
		return err
	}
	pending := make(chan [][]byte, 1)
	defer close(pending)
	ctx, ca := context.WithCancel(ctx)
	defer ca()
	go func() {
		for {
			var docs [][]byte
			select {
			case <-ctx.Done():
				lg.With(
					zap.Error(ctx.Err()),
				).Debug("rule discovery stream closing")
				return
			case docs = <-pending:
			}
		RETRY:
			lg.Debug("sending alert rules to gateway")
			for {
				for _, doc := range docs {
					s.remoteWriteClientMu.Lock()
					client := s.remoteWriteClient
					s.remoteWriteClientMu.Unlock()
					if client == nil {
						err = errors.New("not connected")
					} else {
						_, err = client.SyncRules(ctx, &remotewrite.Payload{
							Headers: map[string]string{
								"Content-Type": "application/yaml",
							},
							Contents: doc,
						})
					}
					if err != nil {
						s.conditions.Set(CondRuleSync, StatusFailure, err.Error())
						// retry, unless another update is received from the channel
						lg.With(
							zap.Error(err),
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
				s.conditions.Clear(CondRuleSync, fmt.Sprintf("successfully sent %d alert rules to gateway", len(docs)))
				break
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			lg.With(
				zap.Error(ctx.Err()),
			).Warn("rule discovery stream closing")
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

func (s *RuleStreamer) configureRuleFinder(config *v1beta1.RulesSpec) (notifier.Finder[rules.RuleGroup], error) {
	if pr := config.Discovery.PrometheusRules; pr != nil {
		client, err := util.NewK8sClient(util.ClientOptions{
			Kubeconfig: pr.Kubeconfig,
			Scheme:     apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create k8s client: %w", err)
		}
		finder := rules.NewPrometheusRuleFinder(client,
			rules.WithLogger(s.logger),
			rules.WithNamespaces(pr.SearchNamespaces...),
		)
		return finder, nil
	} else if config.Discovery.Filesystem != nil {
		return rules.NewFilesystemRuleFinder(config.Discovery.Filesystem), nil
	}

	return nil, errors.New("no rule discovery backend provided")
}

func (s *RuleStreamer) streamRuleGroupUpdates(ctx context.Context, config *v1beta1.RulesSpec) (<-chan [][]byte, error) {
	s.logger.Debug("configuring rule discovery")
	finder, err := s.configureRuleFinder(config)
	if err != nil {
		return nil, fmt.Errorf("failed to configure rule discovery: %w", err)
	}
	s.logger.Debug("rule discovery configured")
	searchInterval := time.Minute * 15
	if interval := config.Discovery.Interval; interval != "" {
		duration, err := time.ParseDuration(interval)
		if err != nil {
			return nil, fmt.Errorf("failed to parse discovery interval: %w", err)
		}
		searchInterval = duration
	}
	notifier := notifier.NewPeriodicUpdateNotifier(ctx, finder, searchInterval)
	s.logger.With(
		zap.String("interval", searchInterval.String()),
	).Debug("rule discovery notifier configured")

	notifierC := notifier.NotifyC(ctx)
	s.logger.Debug("starting rule group update notifier")
	groupYamlDocs := make(chan [][]byte, cap(notifierC))
	go func() {
		defer close(groupYamlDocs)
		for {
			ruleGroups, ok := <-notifierC
			if !ok {
				s.logger.Debug("rule discovery channel closed")
				return
			}
			s.logger.Debug("received updated rule groups from discovery")
			go func() {
				groupYamlDocs <- s.marshalRuleGroups(ruleGroups)
			}()
		}
	}()
	return groupYamlDocs, nil
}

func (s *RuleStreamer) marshalRuleGroups(ruleGroups []rules.RuleGroup) [][]byte {
	yamlDocs := make([][]byte, 0, len(ruleGroups))
	for _, ruleGroup := range ruleGroups {
		doc, err := yaml.Marshal(ruleGroup)
		if err != nil {
			s.logger.With(
				zap.Error(err),
				zap.String("group", ruleGroup.Name),
			).Error("failed to marshal rule group")
			continue
		}
		yamlDocs = append(yamlDocs, doc)
	}
	return yamlDocs
}
