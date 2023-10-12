package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	backoffv2 "github.com/lestrrat-go/backoff/v2"
	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/plugins/alerting/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"go.uber.org/zap"
)

var RuleSyncInterval = time.Minute * 2

const (
	CondRuleSync = "Rule Sync"
)

type RuleStreamer struct {
	parentCtx context.Context
	lg        *zap.SugaredLogger

	stopRuleStream context.CancelFunc

	clientMu       sync.RWMutex
	ruleSyncClient rules.RuleSyncClient

	conditions healthpkg.ConditionTracker
	nodeDriver drivers.NodeDriver
}

var _ drivers.ConfigPropagator = (*RuleStreamer)(nil)

func NewRuleStreamer(
	ctx context.Context,
	lg *zap.SugaredLogger,
	ct healthpkg.ConditionTracker,
	nodeDriver drivers.NodeDriver,
) *RuleStreamer {
	return &RuleStreamer{
		parentCtx:      ctx,
		lg:             lg,
		conditions:     ct,
		nodeDriver:     nodeDriver,
		ruleSyncClient: nil,
	}
}

func (r *RuleStreamer) SetClients(
	ruleClient rules.RuleSyncClient,
) {
	r.clientMu.Lock()
	defer r.clientMu.Unlock()
	r.ruleSyncClient = ruleClient
}

func (r *RuleStreamer) isSet() bool {
	r.clientMu.RLock()
	defer r.clientMu.RUnlock()
	return r.ruleSyncClient != nil
}

func (r *RuleStreamer) ConfigureNode(nodeId string, cfg *node.AlertingCapabilityConfig) error {
	return r.configureRuleStreamer(nodeId, cfg)
}

func (r *RuleStreamer) configureRuleStreamer(nodeId string, cfg *node.AlertingCapabilityConfig) error {
	lg := r.lg.With("nodeId", nodeId)
	lg.Debug("alerting capability updated")

	currentlyRunning := r.stopRuleStream != nil
	shouldRun := cfg.GetEnabled()

	startRuleStreamer := func() {
		ctx, ca := context.WithCancel(r.parentCtx)
		r.stopRuleStream = ca
		go r.run(ctx)
	}

	switch {
	case currentlyRunning && shouldRun:
		lg.Debug("restarting rule stream")
		r.stopRuleStream()
		startRuleStreamer()
	case currentlyRunning && !shouldRun:
		lg.Debug("stopping rule stream")
		r.stopRuleStream()
	case !currentlyRunning && shouldRun:
		lg.Debug("starting rule stream")
		startRuleStreamer()
	case !currentlyRunning && !shouldRun:
		lg.Debug("rule sync is disabled")
	}
	return nil
}

func (r *RuleStreamer) sync(ctx context.Context) {
	r.conditions.Set(CondRuleSync, healthpkg.StatusPending, "")

	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(10),
		backoffv2.WithMinInterval(5*time.Millisecond),
		backoffv2.WithMaxInterval(10*time.Millisecond),
		backoffv2.WithMultiplier(1.5),
	)
	b := retrier.Start(ctx)
	for backoffv2.Continue(b) {
		if !r.isSet() {
			r.conditions.Set(CondRuleSync, healthpkg.StatusFailure, "Rule sync client not set")
			r.lg.Warn("rule sync client not yet set")
			continue
		}
		ruleManifest, err := r.nodeDriver.DiscoverRules(ctx)
		if err != nil {
			r.lg.Warn("failed to discover rules", err, "err")
			r.conditions.Set(CondRuleSync, healthpkg.StatusFailure, fmt.Sprintf("Failed to discover rules : %s", err))
			continue
		}

		r.clientMu.RLock()
		_, err = r.ruleSyncClient.SyncRules(ctx, ruleManifest)
		r.clientMu.RUnlock()
		if err == nil {
			r.conditions.Clear(CondRuleSync)
			r.lg.Infof("successfully synced (%d) rules with gateway", len(ruleManifest.GetRules()))
			break
		}
		r.lg.Warn("failed to sync rules with gateway", err, "err")
		r.conditions.Set(CondRuleSync, healthpkg.StatusFailure, fmt.Sprintf("Failed to sync rules : %s", err))
	}
}

func (r *RuleStreamer) run(ctx context.Context) {
	r.lg.Info("starting initial sync...")
	r.sync(ctx)
	t := time.NewTicker(RuleSyncInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			r.sync(ctx)
		case <-ctx.Done():
			r.lg.Info("Exiting rule sync loop")
			return
		}
	}
}
