/*
- Functions that handle each endpoint implementation update case
- Functions that handle each alert condition case
*/
package alerting

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rancher/opni/pkg/alerting/drivers/cortex"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/messaging"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

func (p *Plugin) setupCondition(
	ctx context.Context,
	_ *zap.SugaredLogger,
	req *alertingv1.AlertCondition,
	newConditionId string) (*corev1.Reference, error) {
	if req.GetAlertType().GetSystem() != nil {
		err := p.handleSystemAlertCreation(ctx, req, newConditionId, req.GetName(), req.Namespace())
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.GetAlertType().GetDownstreamCapability() != nil {
		err := p.handleDownstreamCapabilityAlertCreation(ctx, req, newConditionId, req.GetName(), req.Namespace())
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.GetAlertType().GetMonitoringBackend() != nil {
		err := p.handleMonitoringBackendAlertCreation(ctx, req, newConditionId, req.GetName(), req.Namespace())
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.GetAlertType().GetKubeState() != nil {
		err := p.handleKubeAlertCreation(ctx, req, newConditionId, req.Name)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.GetAlertType().GetCpu() != nil {
		err := p.handleCpuSaturationAlertCreation(ctx, req, newConditionId, req.Name)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.AlertType.GetMemory() != nil {
		err := p.handleMemorySaturationAlertCreation(ctx, req, newConditionId, req.Name)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.AlertType.GetFs() != nil {
		if err := p.handleFsSaturationAlertCreation(ctx, req, newConditionId, req.Name); err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.AlertType.GetPrometheusQuery() != nil {
		if err := p.handlePrometheusQueryAlertCreation(ctx, req, newConditionId, req.Name); err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) deleteCondition(ctx context.Context, _ *zap.SugaredLogger, req *alertingv1.AlertCondition, id string) error {
	if r := req.GetAlertType().GetSystem(); r != nil {
		p.msgNode.RemoveConfigListener(id)
		p.storageClientSet.Get().Incidents().Delete(ctx, id)
		p.storageClientSet.Get().States().Delete(ctx, id)
		return nil
	}
	if r := req.AlertType.GetDownstreamCapability(); r != nil {
		p.msgNode.RemoveConfigListener(id)
		p.storageClientSet.Get().Incidents().Delete(ctx, id)
		p.storageClientSet.Get().States().Delete(ctx, id)
		return nil
	}
	if r := req.AlertType.GetMonitoringBackend(); r != nil {
		p.msgNode.RemoveConfigListener(id)
		p.storageClientSet.Get().Incidents().Delete(ctx, id)
		p.storageClientSet.Get().States().Delete(ctx, id)
		return nil
	}
	if r, _ := handleSwitchCortexRules(req.AlertType); r != nil {
		_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.DeleteRuleRequest{
			ClusterId: r.Id,
			Namespace: shared.OpniAlertingCortexNamespace,
			GroupName: cortex.RuleIdFromUuid(id),
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
	_ context.Context,
	k *alertingv1.AlertCondition,
	newConditionId string,
	conditionName string,
	namespace string,
) error {
	err := p.onSystemConditionCreate(newConditionId, conditionName, namespace, k)
	if err != nil {
		p.Logger.Errorf("failed to create agent condition %s", err)
	}
	return nil
}

func (p *Plugin) handleDownstreamCapabilityAlertCreation(
	_ context.Context,
	k *alertingv1.AlertCondition,
	newConditionId string,
	conditionName string,
	namespace string,
) error {
	err := p.onDownstreamCapabilityConditionCreate(newConditionId, conditionName, namespace, k)
	if err != nil {
		p.Logger.Errorf("failed to create agent condition %s", err)
	}
	return nil
}

func (p *Plugin) handleMonitoringBackendAlertCreation(
	_ context.Context,
	k *alertingv1.AlertCondition,
	newConditionId string,
	conditionName string,
	namespace string,
) error {
	err := p.onCortexClusterStatusCreate(newConditionId, conditionName, namespace, k)
	if err != nil {
		p.Logger.Errorf("failed to create cortex cluster condition %s", err)
	}
	return nil
}

func (p *Plugin) handleKubeAlertCreation(ctx context.Context, cond *alertingv1.AlertCondition, newId, alertName string) error {
	k := cond.GetAlertType().GetKubeState()
	baseKubeRule, err := metrics.NewKubeStateRule(
		k.GetObjectType(),
		k.GetObjectName(),
		k.GetNamespace(),
		k.GetState(),
		cortex.TimeDurationToPromStr(k.GetFor().AsDuration()),
		metrics.KubeStateAnnotations,
	)
	if err != nil {
		return err
	}
	kubeRuleContent, err := cortex.NewPrometheusAlertingRule(newId, alertName,
		cond.GetRoutingLabels(),
		cond.GetRoutingAnnotations(),
		k, nil, baseKubeRule,
	)
	p.Logger.With("handler", "kubeStateAlertCreate").Debugf("kube state alert created %v", kubeRuleContent)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(kubeRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "kube-state").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   k.GetClusterId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out,
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) handleCpuSaturationAlertCreation(
	ctx context.Context,
	cond *alertingv1.AlertCondition,
	conditionId,
	alertName string,
) error {
	c := cond.GetAlertType().GetCpu()
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
	cpuRuleContent, err := cortex.NewPrometheusAlertingRule(conditionId, alertName,
		cond.GetRoutingLabels(),
		cond.GetRoutingAnnotations(),
		c, nil, baseCpuRule)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(cpuRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "cpu").Debugf("%s", string(out))

	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   c.ClusterId.GetId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out,
	})
	return err
}

func (p *Plugin) handleMemorySaturationAlertCreation(ctx context.Context, cond *alertingv1.AlertCondition, conditionId, alertName string) error {
	m := cond.GetAlertType().GetMemory()
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
	memRuleContent, err := cortex.NewPrometheusAlertingRule(conditionId, alertName,
		cond.GetRoutingLabels(),
		cond.GetRoutingAnnotations(),
		m,
		nil,
		baseMemRule,
	)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(memRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "mem").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   m.ClusterId.GetId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out,
	})
	return err
}

func (p *Plugin) handleFsSaturationAlertCreation(ctx context.Context, cond *alertingv1.AlertCondition, conditionId, alertName string) error {
	fs := cond.GetAlertType().GetFs()
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
	fsRuleContent, err := cortex.NewPrometheusAlertingRule(
		conditionId,
		alertName,
		cond.GetRoutingLabels(),
		cond.GetRoutingAnnotations(),
		fs,
		nil,
		baseFsRule,
	)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(fsRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "fs").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   fs.ClusterId.GetId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out,
	})
	return err
}

func (p *Plugin) handlePrometheusQueryAlertCreation(ctx context.Context, cond *alertingv1.AlertCondition, conditionId, alertName string) error {
	q := cond.GetAlertType().GetPrometheusQuery()
	dur := model.Duration(q.GetFor().AsDuration())
	baseRule := &metrics.AlertingRule{
		Alert:       "",
		Expr:        metrics.PostProcessRuleString(q.GetQuery()),
		For:         dur,
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}

	baseRuleContent, err := cortex.NewPrometheusAlertingRule(conditionId, alertName,
		cond.GetRoutingLabels(),
		cond.GetRoutingAnnotations(),
		q, nil, baseRule)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	err = encoder.Encode(baseRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "user-query").Debugf("%s", string(out.Bytes()))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   q.ClusterId.GetId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out.Bytes(),
	})

	return err
}

func (p *Plugin) onSystemConditionCreate(conditionId, conditionName, namespace string, condition *alertingv1.AlertCondition) error {
	lg := p.Logger.With("onSystemConditionCreate", conditionId)
	lg.Debugf("received condition update: %v", condition)
	disconnect := condition.GetAlertType().GetSystem()
	jsCtx, cancel := context.WithCancel(p.Ctx)
	lg.Debugf("Creating agent disconnect with timeout %s", disconnect.GetTimeout().AsDuration())
	agentId := condition.GetClusterId().Id
	evaluator := NewInternalConditionEvaluator(
		&internalConditionMetadata{
			conditionId:        conditionId,
			conditionName:      conditionName,
			lg:                 lg,
			clusterId:          agentId,
			alertmanagerlabels: map[string]string{},
		},
		&internalConditionContext{
			parentCtx:        p.Ctx,
			evaluationCtx:    jsCtx,
			evaluateInterval: time.Second * 10,
			cancelEvaluation: cancel,
			evaluateDuration: disconnect.GetTimeout().AsDuration(),
		},
		&internalConditionStorage{
			js:               p.js.Get(),
			durableConsumer:  NewAgentDurableReplayConsumer(agentId),
			streamSubject:    NewAgentStreamSubject(agentId),
			storageClientSet: p.storageClientSet.Get(),
			msgCh:            make(chan *nats.Msg, 32),
		},
		&internalConditionState{},
		&internalConditionHooks[*corev1.ClusterHealthStatus]{
			healthOnMessage: func(h *corev1.ClusterHealthStatus) (health bool, ts *timestamppb.Timestamp) {
				lg.Debugf("received agent health update connected %v : %s", h.HealthStatus.Status.Connected, h.HealthStatus.Status.Timestamp.String())
				return h.HealthStatus.Status.Connected, h.HealthStatus.Status.Timestamp
			},
			triggerHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				p.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
			},
			resolveHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				_, _ = p.ResolveAlerts(ctx, &alertingv1.ResolveAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
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
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.EvaluateLoop()
	}()
	p.msgNode.AddSystemConfigListener(conditionId, messaging.EvaluatorContext{
		Ctx:    evaluator.evaluationCtx,
		Cancel: evaluator.cancelEvaluation,
	})
	return nil
}

func (p *Plugin) onDownstreamCapabilityConditionCreate(conditionId, conditionName, namespace string, condition *alertingv1.AlertCondition) error {
	lg := p.Logger.With("onCapabilityStatusCreate", conditionId)
	capability := condition.GetAlertType().GetDownstreamCapability()
	lg.Debugf("received condition update: %v", condition)
	jsCtx, cancel := context.WithCancel(p.Ctx)
	lg.Debugf("Creating agent capability unhealthy with timeout %s", capability.GetFor().AsDuration())
	agentId := condition.GetClusterId().Id
	evaluator := NewInternalConditionEvaluator(
		&internalConditionMetadata{
			conditionId:        conditionId,
			conditionName:      conditionName,
			lg:                 lg,
			clusterId:          agentId,
			alertmanagerlabels: map[string]string{},
		},
		&internalConditionContext{
			parentCtx:        p.Ctx,
			evaluationCtx:    jsCtx,
			evaluateInterval: time.Second * 10,
			cancelEvaluation: cancel,
			evaluateDuration: capability.GetFor().AsDuration(),
		},
		&internalConditionStorage{
			js:               p.js.Get(),
			durableConsumer:  NewAgentDurableReplayConsumer(agentId),
			streamSubject:    NewAgentStreamSubject(agentId),
			storageClientSet: p.storageClientSet.Get(),
			msgCh:            make(chan *nats.Msg, 32),
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
					for _, badState := range capability.GetCapabilityState() {
						if strings.Contains(s, badState) {
							healthy = false
							break
						}
					}
				}
				return healthy, h.HealthStatus.Status.Timestamp
			},
			triggerHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				_, _ = p.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
			},
			resolveHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				_, _ = p.ResolveAlerts(ctx, &alertingv1.ResolveAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
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
		defer cancel() // cancel parent context, if we return (non-recoverable)
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

func (p *Plugin) onCortexClusterStatusCreate(conditionId, conditionName, namespace string, condition *alertingv1.AlertCondition) error {
	lg := p.Logger.With("onCortexClusterStatusCreate", conditionId)
	cortex := condition.GetAlertType().GetMonitoringBackend()
	lg.Debugf("received condition update: %v", condition)
	jsCtx, cancel := context.WithCancel(p.Ctx)
	lg.Debugf("Creating cortex status with timeout %s", cortex.GetFor().AsDuration())

	evaluator := NewInternalConditionEvaluator(
		&internalConditionMetadata{
			conditionId:        conditionId,
			conditionName:      conditionName,
			lg:                 lg,
			clusterId:          "", // unused here
			alertmanagerlabels: map[string]string{},
		},
		&internalConditionContext{
			parentCtx:        p.Ctx,
			evaluationCtx:    jsCtx,
			evaluateInterval: time.Minute,
			cancelEvaluation: cancel,
			evaluateDuration: cortex.GetFor().AsDuration(),
		},
		&internalConditionStorage{
			js:               p.js.Get(),
			durableConsumer:  nil,
			streamSubject:    NewCortexStatusSubject(),
			storageClientSet: p.storageClientSet.Get(),
			msgCh:            make(chan *nats.Msg, 32),
		},
		&internalConditionState{},
		&internalConditionHooks[*cortexadmin.CortexStatus]{
			healthOnMessage: func(h *cortexadmin.CortexStatus) (healthy bool, ts *timestamppb.Timestamp) {
				return reduceCortexAdminStates(cortex.GetBackendComponents(), h)
			},
			triggerHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				_, _ = p.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
			},
			resolveHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				lg.Debug("resolve cortex status condition")
				_, _ = p.ResolveAlerts(ctx, &alertingv1.ResolveAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
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
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.EvaluateLoop()
	}()
	p.msgNode.AddSystemConfigListener(conditionId, messaging.EvaluatorContext{
		Ctx:    evaluator.evaluationCtx,
		Cancel: evaluator.cancelEvaluation,
	})
	return nil
}
