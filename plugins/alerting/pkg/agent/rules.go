package agent

import (
	"context"
	"fmt"
	"time"

	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
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
	util.Initializer

	parentCtx context.Context
	lg        *zap.SugaredLogger

	stopRuleStream context.CancelFunc
	ruleSyncClient future.Future[rules.RuleSyncClient]

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
		ruleSyncClient: future.New[rules.RuleSyncClient](),
	}
}

func (r *RuleStreamer) Initialize(ruleSyncClient rules.RuleSyncClient) {
	r.InitOnce(func() {
		r.ruleSyncClient.Set(ruleSyncClient)
	})
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
	ruleManifest, err := r.nodeDriver.DiscoverRules(ctx)
	if err != nil {
		r.conditions.Set(CondRuleSync, healthpkg.StatusFailure, fmt.Sprintf("Failed to discover rules : %s", err))
		r.lg.Warnf("failed to discover rules %s", err)
		return
	}
	r.lg.Infof("discovered %d rules", len(ruleManifest.Rules))
	ctx, ca := context.WithTimeout(ctx, RuleSyncInterval)
	defer ca()
	syncClient, err := r.ruleSyncClient.GetContext(ctx)
	if err != nil {
		r.conditions.Set(CondRuleSync, healthpkg.StatusFailure, fmt.Sprintf("Failed to get rule sync client : %s", err))
		r.lg.Error("failed to get rule sync client", err, "err")
		return
	}
	if _, err := syncClient.SyncRules(ctx, ruleManifest); err != nil {
		r.conditions.Set(CondRuleSync, healthpkg.StatusFailure, fmt.Sprintf("Failed to sync rules : %s", err))
		r.lg.Warnf("failed to sync rules %s", err)
	} else {
		r.conditions.Clear(CondRuleSync)
	}
}

func (r *RuleStreamer) run(ctx context.Context) {
	r.lg.Info("waiting for rule sync client...")
	if err := r.WaitForInitContext(ctx); err != nil {
		r.conditions.Set(CondRuleSync, healthpkg.StatusDisabled, fmt.Sprintf("Failed to run the rule syncer : %s", err))
		r.lg.Errorf("failed to wait for rule sync client %s", err)
		return
	}
	r.lg.Info("rule sync client acquired, starting initial sync...")
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
