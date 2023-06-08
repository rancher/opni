package agent

import (
	"context"

	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/plugins/alerting/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"go.uber.org/zap"
)

type RuleStreamer struct {
	parentCtx context.Context

	lg *zap.SugaredLogger

	ruleStreamCtx  context.Context
	stopRuleStream context.CancelFunc

	conditions healthpkg.ConditionTracker
}

var _ drivers.ConfigPropagator = (*RuleStreamer)(nil)

func NewRuleStreamer(ctx context.Context, lg *zap.SugaredLogger, ct healthpkg.ConditionTracker) *RuleStreamer {
	return &RuleStreamer{
		parentCtx:  ctx,
		lg:         lg,
		conditions: ct,
	}
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
		// TODO : iterate over drivers and configure rule discoverers with rule spec

		go r.run(ctx) // TODO : run with all the rule discoverers
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

func (r *RuleStreamer) run(ctx context.Context) {
	// TODO:
}
