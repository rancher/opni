/*
- Functions that handle each endpoint implementation update case
- Functions that handle each alert condition case
*/
package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/modeltraining"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/messaging"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

type modelTrainingInfo struct {
	Minutes  int64
	Epochs   int64
	Accuracy float64
}

func setupCondition(
	p *Plugin,
	lg *zap.SugaredLogger,
	ctx context.Context,
	req *alertingv1.AlertCondition,
	newConditionId string) (*corev1.Reference, error) {
	if s := req.GetAlertType().GetSystem(); s != nil {
		err := p.handleSystemAlertCreation(ctx, s, newConditionId, req.GetName())
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if dc := req.GetAlertType().GetDownstreamCapability(); dc != nil {
		err := p.handleDownstreamCapabilityAlertCreation(ctx, dc, newConditionId, req.GetName())
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if cs := req.GetAlertType().GetMonitoringBackend(); cs != nil {
		err := p.handleMonitoringBackendAlertCreation(ctx, cs, newConditionId, req.GetName())
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if mt := req.GetAlertType().GetModelTrainingStatus(); mt != nil {
		err := p.handleModelTrainingStatusAlertCreation(ctx, mt, newConditionId, req.GetName())
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if k := req.GetAlertType().GetKubeState(); k != nil {
		err := p.handleKubeAlertCreation(ctx, k, newConditionId, req.Name)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if c := req.GetAlertType().GetCpu(); c != nil {
		err := p.handleCpuSaturationAlertCreation(ctx, c, newConditionId, req.Name)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if m := req.AlertType.GetMemory(); m != nil {
		err := p.handleMemorySaturationAlertCreation(ctx, m, newConditionId, req.Name)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if fs := req.AlertType.GetFs(); fs != nil {
		if err := p.handleFsSaturationAlertCreation(ctx, fs, newConditionId, req.Name); err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if q := req.AlertType.GetPrometheusQuery(); q != nil {
		if err := p.handlePrometheusQueryAlertCreation(ctx, q, newConditionId, req.Name); err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	return nil, shared.AlertingErrNotImplemented
}

func deleteCondition(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, req *alertingv1.AlertCondition, id string) error {
	if r := req.GetAlertType().GetSystem(); r != nil {
		p.msgNode.RemoveConfigListener(id)
		p.storageNode.DeleteIncidentTracker(ctx, id)
		p.storageNode.DeleteConditionStatusTracker(ctx, id)
		return nil
	}
	if r := req.AlertType.GetDownstreamCapability(); r != nil {
		p.msgNode.RemoveConfigListener(id)
		p.storageNode.DeleteIncidentTracker(ctx, id)
		p.storageNode.DeleteConditionStatusTracker(ctx, id)
		return nil
	}
	if r := req.AlertType.GetMonitoringBackend(); r != nil {
		p.msgNode.RemoveConfigListener(id)
		p.storageNode.DeleteIncidentTracker(ctx, id)
		p.storageNode.DeleteConditionStatusTracker(ctx, id)
		return nil
	}
	if req.AlertType.GetModelTrainingStatus() != nil {
		p.msgNode.RemoveConfigListener(id)
		p.storageNode.DeleteIncidentTracker(ctx, id)
		p.storageNode.DeleteConditionStatusTracker(ctx, id)
		return nil
	}
	if r, _ := handleSwitchCortexRules(req.AlertType); r != nil {
		_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
			ClusterId: r.Id,
			GroupName: CortexRuleIdFromUuid(id),
		})
		return err
	}
	return shared.AlertingErrNotImplemented
}

func handleSwitchCortexRules(t *alertingv1.AlertTypeDetails) (*corev1.Reference, alertingv1.IndexableMetric) {
	if k := t.GetKubeState(); k != nil {
		return &corev1.Reference{Id: k.ClusterId}, k
	}
	if c := t.GetCpu(); c != nil {
		return c.ClusterId, c
	}
	if m := t.GetMemory(); m != nil {
		return m.ClusterId, m
	}
	if f := t.GetFs(); f != nil {
		return f.ClusterId, f
	}
	if q := t.GetPrometheusQuery(); q != nil {
		return q.ClusterId, q
	}

	return nil, nil
}

func (p *Plugin) handleSystemAlertCreation(
	ctx context.Context,
	k *alertingv1.AlertConditionSystem,
	newConditionId string,
	conditionName string,
) error {
	err := p.onSystemConditionCreate(newConditionId, conditionName, k)
	if err != nil {
		p.Logger.Errorf("failed to create agent condition %s", err)
	}
	return nil
}

func (p *Plugin) handleDownstreamCapabilityAlertCreation(
	ctx context.Context,
	k *alertingv1.AlertConditionDownstreamCapability,
	newConditionId string,
	conditionName string,
) error {
	err := p.onDownstreamCapabilityConditionCreate(newConditionId, conditionName, k)
	if err != nil {
		p.Logger.Errorf("failed to create agent condition %s", err)
	}
	return nil
}

func (p *Plugin) handleModelTrainingStatusAlertCreation(
	ctx context.Context,
	k *alertingv1.AlertConditionModelTrainingStatus,
	newConditionId string,
	conditionName string,
) error {
	err := p.onModelTrainingStatusConditionCreate(newConditionId, conditionName, k)
	if err != nil {
		p.Logger.Errorf("failed to create agent condition %s", err)
	}
	return nil
}

func (p *Plugin) handleMonitoringBackendAlertCreation(
	ctx context.Context,
	k *alertingv1.AlertConditionMonitoringBackend,
	newConditionId string,
	conditionName string,
) error {
	err := p.onCortexClusterStatusCreate(newConditionId, conditionName, k)
	if err != nil {
		p.Logger.Errorf("failed to create cortex cluster condition %s", err)
	}
	return nil
}

func (p *Plugin) handleKubeAlertCreation(ctx context.Context, k *alertingv1.AlertConditionKubeState, newId, alertName string) error {
	baseKubeRule, err := metrics.NewKubeStateRule(
		k.GetObjectType(),
		k.GetObjectName(),
		k.GetNamespace(),
		k.GetState(),
		timeDurationToPromStr(k.GetFor().AsDuration()),
		metrics.KubeStateAnnotations,
	)
	if err != nil {
		return err
	}
	kubeRuleContent, err := NewCortexAlertingRule(newId, alertName, k, nil, baseKubeRule)
	p.Logger.With("handler", "kubeStateAlertCreate").Debugf("kube state alert created %v", kubeRuleContent)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(kubeRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "kube-state").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		ClusterId:   k.GetClusterId(),
		YamlContent: string(out),
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) handleCpuSaturationAlertCreation(
	ctx context.Context,
	c *alertingv1.AlertConditionCPUSaturation,
	conditionId,
	alertName string,
) error {
	baseCpuRule, err := metrics.NewCpuRule(
		c.GetNodeCoreFilters(),
		c.GetCpuStates(),
		c.GetOperation(),
		float64(c.GetExpectedRatio()),
		c.GetFor(),
		metrics.CpuRuleAnnotations,
	)
	if err != nil {
		return err
	}
	cpuRuleContent, err := NewCortexAlertingRule(conditionId, alertName, c, nil, baseCpuRule)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(cpuRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "cpu").Debugf("%s", string(out))

	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		ClusterId:   c.ClusterId.GetId(),
		YamlContent: string(out),
	})
	return err
}

func (p *Plugin) handleMemorySaturationAlertCreation(ctx context.Context, m *alertingv1.AlertConditionMemorySaturation, conditionId, alertName string) error {
	baseMemRule, err := metrics.NewMemRule(
		m.GetNodeMemoryFilters(),
		m.UsageTypes,
		m.GetOperation(),
		float64(m.GetExpectedRatio()),
		m.GetFor(),
		metrics.MemRuleAnnotations,
	)
	if err != nil {
		return err
	}
	memRuleContent, err := NewCortexAlertingRule(conditionId, alertName, m, nil, baseMemRule)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(memRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "mem").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		ClusterId:   m.ClusterId.GetId(),
		YamlContent: string(out),
	})
	return err
}

func (p *Plugin) handleFsSaturationAlertCreation(ctx context.Context, fs *alertingv1.AlertConditionFilesystemSaturation, conditionId, alertName string) error {
	baseFsRule, err := metrics.NewFsRule(
		fs.GetNodeFilters(),
		fs.GetOperation(),
		float64(fs.GetExpectedRatio()),
		fs.GetFor(),
		metrics.MemRuleAnnotations,
	)
	if err != nil {
		return err
	}
	fsRuleContent, err := NewCortexAlertingRule(conditionId, alertName, fs, nil, baseFsRule)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(fsRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "fs").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		ClusterId:   fs.ClusterId.GetId(),
		YamlContent: string(out),
	})
	return err
}

func (p *Plugin) handlePrometheusQueryAlertCreation(ctx context.Context, q *alertingv1.AlertConditionPrometheusQuery, conditionId, alertName string) error {
	dur := model.Duration(q.GetFor().AsDuration())
	baseRule := &metrics.AlertingRule{
		Alert:       "",
		Expr:        metrics.PostProcessRuleString(q.GetQuery()),
		For:         dur,
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}

	baseRuleContent, err := NewCortexAlertingRule(conditionId, alertName, q, nil, baseRule)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(baseRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "user-query").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		ClusterId:   q.ClusterId.GetId(),
		YamlContent: string(out),
	})

	return err
}

func (p *Plugin) onSystemConditionCreate(conditionId, conditionName string, condition *alertingv1.AlertConditionSystem) error {
	lg := p.Logger.With("onSystemConditionCreate", conditionId)
	lg.Debugf("received condition update: %v", condition)
	jsCtx, cancel := context.WithCancel(p.Ctx)
	lg.Debugf("Creating agent disconnect with timeout %s", condition.GetTimeout().AsDuration())
	agentId := condition.GetClusterId().Id

	evaluator := NewInternalConditionEvaluator(
		&internalConditionMetadata{
			conditionId:        conditionId,
			conditionName:      conditionName,
			lg:                 lg,
			clusterId:          agentId,
			alertmanagerlabels: condition.GetTriggerAnnotations(),
		},
		&internalConditionContext{
			parentCtx:        p.Ctx,
			evaluationCtx:    jsCtx,
			evaluateInterval: time.Second * 10,
			cancelEvaluation: cancel,
			evaluateDuration: condition.GetTimeout().AsDuration(),
		},
		&internalConditionStorage{
			js:              p.js.Get(),
			durableConsumer: NewAgentDurableReplayConsumer(agentId),
			streamSubject:   NewAgentStreamSubject(agentId),
			storageNode:     p.storageNode,
			msgCh:           make(chan *nats.Msg, 32),
		},
		&internalConditionState{},
		&internalConditionHooks[*corev1.ClusterHealthStatus]{
			healthOnMessage: func(h *corev1.ClusterHealthStatus) (health bool, ts *timestamppb.Timestamp) {
				lg.Debugf("received agent health update connected %v : %s", h.HealthStatus.Status.Connected, h.HealthStatus.Status.Timestamp.String())
				return h.HealthStatus.Status.Connected, h.HealthStatus.Status.Timestamp
			},
			finalizerOnMessage: func(h *corev1.ClusterHealthStatus) (done bool) {
				return false
			},
			triggerHook: func(ctx context.Context, conditionId string, labels map[string]string) {
				p.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
					ConditionId: &corev1.Reference{Id: conditionId},
					Annotations: labels,
				})
			},
			resolveHook: func(ctx context.Context, conditionId string, labels map[string]string) {
				_, _ = p.ResolveAlerts(ctx, &alertingv1.ResolveAlertsRequest{
					ConditionId: &corev1.Reference{Id: conditionId},
					Annotations: labels,
				})
			},
			finalizerHook: func(ctx context.Context, msg *corev1.ClusterHealthStatus, conditionId string, labels map[string]string) {
			},
		},
	)
	// handles re-entrant conditions
	evaluator.CalculateInitialState()
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.SubscriberLoop()
	}()
	// spawn a watcher for the incidents
	go func() {
		defer cancel()
		evaluator.EvaluateLoop()
	}()
	p.msgNode.AddSystemConfigListener(conditionId, messaging.EvaluatorContext{
		Ctx:    evaluator.evaluationCtx,
		Cancel: evaluator.cancelEvaluation,
	})
	return nil
}

func (p *Plugin) onDownstreamCapabilityConditionCreate(conditionId, conditionName string, condition *alertingv1.AlertConditionDownstreamCapability) error {
	lg := p.Logger.With("onCapabilityStatusCreate", conditionId)
	lg.Debugf("received condition update: %v", condition)
	jsCtx, cancel := context.WithCancel(p.Ctx)
	lg.Debugf("Creating agent capability unhealthy with timeout %s", condition.GetFor().AsDuration())
	agentId := condition.GetClusterId().Id
	evaluator := NewInternalConditionEvaluator(
		&internalConditionMetadata{
			conditionId:        conditionId,
			conditionName:      conditionName,
			lg:                 lg,
			clusterId:          agentId,
			alertmanagerlabels: condition.GetTriggerAnnotations(),
		},
		&internalConditionContext{
			parentCtx:        p.Ctx,
			evaluationCtx:    jsCtx,
			evaluateInterval: time.Second * 10,
			cancelEvaluation: cancel,
			evaluateDuration: condition.GetFor().AsDuration(),
		},
		&internalConditionStorage{
			js:              p.js.Get(),
			durableConsumer: NewAgentDurableReplayConsumer(agentId),
			streamSubject:   NewAgentStreamSubject(agentId),
			storageNode:     p.storageNode,
			msgCh:           make(chan *nats.Msg, 32),
		},
		&internalConditionState{},
		&internalConditionHooks[*corev1.ClusterHealthStatus]{
			healthOnMessage: func(h *corev1.ClusterHealthStatus) (healthy bool, ts *timestamppb.Timestamp) {
				healthy = true
				if h.HealthStatus.Health == nil {
					return false, h.HealthStatus.Status.Timestamp
				}
				lg.Debugf("found health conditions %v", h.HealthStatus.Health.Conditions)
				for _, s := range h.HealthStatus.Health.Conditions {
					for _, badState := range condition.GetCapabilityState() {
						if strings.Contains(s, badState) {
							healthy = false
							break
						}
					}

				}
				return healthy, h.HealthStatus.Status.Timestamp
			},
			finalizerOnMessage: func(h *corev1.ClusterHealthStatus) (done bool) {
				return false
			},
			triggerHook: func(ctx context.Context, conditionId string, labels map[string]string) {
				_, _ = p.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
					ConditionId: &corev1.Reference{Id: conditionId},
					Annotations: labels,
				})
			},
			resolveHook: func(ctx context.Context, conditionId string, labels map[string]string) {
				_, _ = p.ResolveAlerts(ctx, &alertingv1.ResolveAlertsRequest{
					ConditionId: &corev1.Reference{Id: conditionId},
					Annotations: labels,
				})
			},
			finalizerHook: func(ctx context.Context, msg *corev1.ClusterHealthStatus, conditionId string, labels map[string]string) {
			},
		},
	)
	// handles re-entrant conditions
	evaluator.CalculateInitialState()
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.SubscriberLoop()
	}()
	// spawn a watcher for the incidents
	go func() {
		defer cancel()
		evaluator.EvaluateLoop()
	}()
	p.msgNode.AddSystemConfigListener(conditionId, messaging.EvaluatorContext{
		Ctx:    evaluator.evaluationCtx,
		Cancel: evaluator.cancelEvaluation,
	})
	return nil
}

func reduceCortexAdminStates(componentsToTrack []string, cStatus *cortexadmin.CortexStatus) (healthy bool, ts *timestamppb.Timestamp) {
	if cStatus == nil {
		return false, timestamppb.Now()
	}
	ts = cStatus.GetTimestamp()
	// helps track status errors to particular components, like having 3 expected replicas, but only 1-2 are running
	memberReports := map[string]bool{}
	for _, cmp := range componentsToTrack {
		switch cmp {
		case shared.CortexDistributor:
			if cStatus.Distributor == nil {
				return false, ts
			}
			for _, svc := range cStatus.Distributor.GetServices().GetServices() {
				memberReports[svc.GetName()] = true
				if svc.GetStatus() != "Running" {
					return false, ts
				}
			}
		case shared.CortexIngester:
			if cStatus.Ingester == nil {
				return false, ts
			}
			for _, member := range cStatus.Ingester.Memberlist.Members.Items {
				if _, ok := memberReports[member.Name]; !ok {
					memberReports[member.Name] = true
				}
			}
			for _, svc := range cStatus.Ingester.GetServices().GetServices() {
				memberReports[svc.GetName()] = true
				if svc.GetStatus() != "Running" {
					return false, ts
				}
			}
		case shared.CortexRuler:
			if cStatus.Ruler == nil {
				return false, ts
			}
			for _, member := range cStatus.Ruler.Memberlist.Members.Items {
				if _, ok := memberReports[member.Name]; !ok {
					memberReports[member.Name] = true
				}
			}
			for _, svc := range cStatus.Ruler.GetServices().GetServices() {
				memberReports[svc.GetName()] = true
				if svc.GetStatus() != "Running" {
					return false, ts
				}
			}
		case shared.CortexPurger:
			if cStatus.Purger == nil {
				return false, ts
			}
			for _, svc := range cStatus.Purger.GetServices().GetServices() {
				memberReports[svc.GetName()] = true
				if svc.GetStatus() != "Running" {
					return false, ts
				}
			}
		case shared.CortexCompactor:
			if cStatus.Compactor == nil {
				return false, ts
			}
			for _, member := range cStatus.Compactor.Memberlist.Members.Items {
				if _, ok := memberReports[member.Name]; !ok {
					memberReports[member.Name] = true
				}
			}
			for _, svc := range cStatus.Compactor.GetServices().GetServices() {
				memberReports[svc.GetName()] = true
				if svc.GetStatus() != "Running" {
					return false, ts
				}
			}
		case shared.CortexStoreGateway:
			if cStatus.StoreGateway == nil {
				return false, ts
			}
			for _, svc := range cStatus.StoreGateway.GetServices().GetServices() {
				memberReports[svc.GetName()] = true
				if svc.GetStatus() != "Running" {
					return false, ts
				}
			}
		case shared.CortexQueryFrontend:
			if cStatus.QueryFrontend == nil {
				return false, ts
			}
			for _, svc := range cStatus.QueryFrontend.GetServices().GetServices() {
				memberReports[svc.GetName()] = true
				if svc.GetStatus() != "Running" {
					return false, ts
				}
			}
		case shared.CortexQuerier:
			if cStatus.Querier == nil {
				return false, ts
			}
			for _, svc := range cStatus.Querier.GetServices().GetServices() {
				memberReports[svc.GetName()] = true
				if svc.GetStatus() != "Running" {
					return false, ts
				}
			}
		}
	}
	// on cortex-status error, if a specific component is not reported, we assume it is unhealthy
	for _, component := range componentsToTrack {
		for member, reportedOn := range memberReports {
			if strings.Contains(member, component) && !reportedOn {
				return false, ts
			}
		}
	}
	return true, ts
}

func (p *Plugin) onCortexClusterStatusCreate(conditionId, conditionName string, condition *alertingv1.AlertConditionMonitoringBackend) error {
	lg := p.Logger.With("onCortexClusterStatusCreate", conditionId)
	lg.Debugf("received condition update: %v", condition)
	jsCtx, cancel := context.WithCancel(p.Ctx)
	lg.Debugf("Creating cortex status with timeout %s", condition.GetFor().AsDuration())

	evaluator := NewInternalConditionEvaluator(
		&internalConditionMetadata{
			conditionId:        conditionId,
			conditionName:      conditionName,
			lg:                 lg,
			clusterId:          "", // unused here
			alertmanagerlabels: condition.GetTriggerAnnotations(),
		},
		&internalConditionContext{
			parentCtx:        p.Ctx,
			evaluationCtx:    jsCtx,
			evaluateInterval: time.Minute,
			cancelEvaluation: cancel,
			evaluateDuration: condition.GetFor().AsDuration(),
		},
		&internalConditionStorage{
			js:              p.js.Get(),
			durableConsumer: nil,
			streamSubject:   NewCortexStatusSubject(),
			storageNode:     p.storageNode,
			msgCh:           make(chan *nats.Msg, 32),
		},
		&internalConditionState{},
		&internalConditionHooks[*cortexadmin.CortexStatus]{
			healthOnMessage: func(h *cortexadmin.CortexStatus) (healthy bool, ts *timestamppb.Timestamp) {
				return reduceCortexAdminStates(condition.GetBackendComponents(), h)
			},
			finalizerOnMessage: func(h *cortexadmin.CortexStatus) (done bool) {
				return false
			},
			triggerHook: func(ctx context.Context, conditionId string, labels map[string]string) {
				_, _ = p.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
					ConditionId: &corev1.Reference{Id: conditionId},
					Annotations: labels,
				})
			},
			resolveHook: func(ctx context.Context, conditionId string, labels map[string]string) {
				lg.Debug("resolve cortex status condition")
				_, _ = p.ResolveAlerts(ctx, &alertingv1.ResolveAlertsRequest{
					ConditionId: &corev1.Reference{Id: conditionId},
					Annotations: labels,
				})
			},
			finalizerHook: func(ctx context.Context, msg *cortexadmin.CortexStatus, conditionId string, labels map[string]string) {
			},
		},
	)
	// handles re-entrant conditions
	evaluator.CalculateInitialState()
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.SubscriberLoop()
	}()
	// spawn a watcher for the incidents
	go func() {
		defer cancel()
		evaluator.EvaluateLoop()
	}()
	p.msgNode.AddSystemConfigListener(conditionId, messaging.EvaluatorContext{
		Ctx:    evaluator.evaluationCtx,
		Cancel: evaluator.cancelEvaluation,
	})
	return nil
}
func (p *Plugin) onModelTrainingStatusConditionCreate(conditionId, conditionName string, condition *alertingv1.AlertConditionModelTrainingStatus) error {
	lg := p.Logger.With("onModelTrainingStatusConditionCreate", conditionId)
	lg.Debugf("received condition update: %v", condition)
	jsCtx, cancel := context.WithCancel(p.Ctx)
	lg.Debugf("Creating model training status with hang duration : %s", condition.GetHangDuration().AsDuration())

	lastPercentage := atomic.NewInt64(0)

	evaluator := NewInternalConditionEvaluator(
		&internalConditionMetadata{
			conditionId:        conditionId,
			conditionName:      conditionName,
			lg:                 lg,
			clusterId:          "", // unused here
			alertmanagerlabels: condition.GetTriggerAnnotations(),
		},
		&internalConditionContext{
			parentCtx:        p.Ctx,
			evaluationCtx:    jsCtx,
			evaluateInterval: time.Second * 15,
			cancelEvaluation: cancel,
			evaluateDuration: condition.GetHangDuration().AsDuration(),
		},
		&internalConditionStorage{
			js:              p.js.Get(),
			durableConsumer: nil,
			streamSubject:   NewModelTrainingStatusSubject(condition.GetJobUuid()),
			storageNode:     p.storageNode,
			msgCh:           make(chan *nats.Msg, 32),
		},
		&internalConditionState{},
		&internalConditionHooks[*modeltraining.ModelStatus]{
			healthOnMessage: func(h *modeltraining.ModelStatus) (healthy bool, ts *timestamppb.Timestamp) {
				if h.GetStatistics().GetLastReportedUpdate() == nil {
					h.GetStatistics().LastReportedUpdate = timestamppb.Now()
				}
				// FIXME: status string subject to change
				if h.GetStatus() == "not started" || h.GetStatistics().LastReportedUpdate == nil {
					return false, h.GetStatistics().LastReportedUpdate
				}
				if lastPercentage.Load() == h.GetStatistics().PercentageCompleted {
					return false, h.GetStatistics().LastReportedUpdate
				}
				lastPercentage.Store(h.GetStatistics().PercentageCompleted)
				return true, nil
			},
			finalizerOnMessage: func(h *modeltraining.ModelStatus) (done bool) {
				//FIXME: status string subject to change
				return h.GetStatistics().PercentageCompleted >= 100 || h.GetStatus() == "completed"
			},
			triggerHook: func(ctx context.Context, conditionId string, labels map[string]string) {
				_, _ = p.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
					ConditionId: &corev1.Reference{Id: conditionId},
					Annotations: labels,
				})
			},
			resolveHook: func(ctx context.Context, conditionId string, labels map[string]string) {
				lg.Debug("resolve cortex status condition")
				_, _ = p.ResolveAlerts(ctx, &alertingv1.ResolveAlertsRequest{
					ConditionId: &corev1.Reference{Id: conditionId},
					Annotations: labels,
				})
			},
			finalizerHook: func(ctx context.Context, finalMsg *modeltraining.ModelStatus, conditionId string, labels map[string]string) {
				// read condition attached endpoints
				deets, err := p.GetAlertCondition(ctx, &corev1.Reference{Id: conditionId})
				if err != nil {
					lg.Warnf("failed to get alert condition : %s", err)
					return
				}
				defer func() {
					// plugin context so it doesn't cause a race condition with cancelling the evaluation
					_, err = p.DeleteAlertCondition(p.Ctx, &corev1.Reference{Id: conditionId})
					if err != nil {
						lg.Warnf("failed to delete alert condition after model training status is done: %s", err)
					}
				}()

				endpoints := []*alertingv1.AlertEndpoint{}
				for _, ep := range deets.GetAttachedEndpoints().GetItems() {
					endp, err := p.GetAlertEndpoint(ctx, &corev1.Reference{Id: ep.GetEndpointId()})
					if err != nil {
						lg.Warnf("cannot find expected alert endpoint %s", ep.GetEndpointId())
						continue
					}
					err = unredactSecrets(ctx, p.storageNode, ep.GetEndpointId(), endp)
					if err != nil {
						lg.Warnf("cannot unredact secrets for alert endpoint %s", ep.GetEndpointId())
						continue
					}
					endpoints = append(endpoints, endp)
				}
				// read info from finalMsg
				details := &alertingv1.EndpointImplementation{
					Title: fmt.Sprintf("Model training done for training job %s", finalMsg.GetStatistics().GetUuid()),
					Body: fmt.Sprintf("Model trained for %d minutes, %d epochs, with an accuracy of %f",
						finalMsg.Statistics.GetTimeElapsed()/60,
						finalMsg.Statistics.GetCurrentEpoch(),
						finalMsg.Statistics.GetModelAccuracy()),
				}

				dispatcher, err := p.EphemeralDispatcher(p.Ctx, &alertingv1.EphemeralDispatcherRequest{
					Ttl:           durationpb.New(time.Duration(time.Minute)),
					NumDispatches: 1,
					Prefix:        "model-training-done",
					Items:         endpoints,
					Details:       details,
				})
				if err != nil {
					lg.Warnf("failed to create ephemeral dispatcher : %s", err)
				} else {
					p.TriggerAlerts(p.Ctx, dispatcher.TriggerAlertsRequest)
					p.deleteRoutingNode(p.Ctx, dispatcher.TriggerAlertsRequest.ConditionId.Id, lg)
				}
			},
		},
	)
	// handles re-entrant conditions
	evaluator.CalculateInitialState()
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.SubscriberLoop()
	}()
	// spawn a watcher for the incidents
	go func() {
		defer cancel()
		evaluator.EvaluateLoop()
	}()
	p.msgNode.AddSystemConfigListener(conditionId, messaging.EvaluatorContext{
		Ctx:    evaluator.evaluationCtx,
		Cancel: evaluator.cancelEvaluation,
	})
	return nil
}
