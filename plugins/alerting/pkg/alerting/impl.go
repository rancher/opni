package alerting

import (
	"context"
	"sync"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/messaging"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AgentDisconnect struct {
	firingLock sync.Mutex
	isFiring   bool
}

func NewAgentDisconnect(
	conditionId string,
	cond *alertingv1.AlertConditionSystem,
	msgNode *messaging.MessagingNode,
) *AgentDisconnect {
	return &AgentDisconnect{}
}

func (a *AgentDisconnect) Create(conditionId string, cond *alertingv1.AlertCondition) {
	// TODO
	panic("implement me")
}

func (a *AgentDisconnect) Delete(conditionId string, cond *alertingv1.AlertCondition) {
	// TODO
	panic("implement me")
}

func (a *AgentDisconnect) Evaluate(parentCtx, ctxWithCancel context.Context, opts ...EvaluatorOptions) {
	// TODO
	panic("implement me")
}

func (a *AgentDisconnect) Ingester(parentCtx, ctxWithCancel context.Context, opts ...EvaluatorOptions) {
	// TODO
	panic("implement me")
}

func (a *AgentDisconnect) Aggregator() {
	// TODO
	panic("implement me")
}

var _ ConditionEvaluator = &AgentDisconnect{}

type AlertTypeSystemBackend struct {
	RequestBase
}

func (a *AlertTypeSystemBackend) ListTemplates() []TemplateInfo {
	return []TemplateInfo{}
}

func (a *AlertTypeSystemBackend) ListChoices() error {
	return nil
}

func (a *AlertTypeSystemBackend) Create(p *Plugin, req alertingv1.AlertCondition) (*corev1.Reference, error) {
	return nil, nil
}

func (a *AlertTypeSystemBackend) Update(p *Plugin, req alertingv1.AlertConditionWithId) (*emptypb.Empty, error) {
	return nil, nil
}

func (a *AlertTypeSystemBackend) Delete(p *Plugin, req *corev1.Reference) (*emptypb.Empty, error) {
	return nil, nil
}

type AlertTypeKubeMetricsBackend struct {
	RequestBase
}

func (a *AlertTypeKubeMetricsBackend) ListTemplates() []TemplateInfo {
	return nil
}

func (a *AlertTypeKubeMetricsBackend) ListChoices() error {
	return nil
}

func (a *AlertTypeKubeMetricsBackend) Create(p *Plugin, req alertingv1.AlertCondition) (*corev1.Reference, error) {
	return nil, nil
}

func (a *AlertTypeKubeMetricsBackend) Update(p *Plugin, req alertingv1.AlertConditionWithId) (*emptypb.Empty, error) {
	return nil, nil
}

func (a *AlertTypeKubeMetricsBackend) Delete(p *Plugin, req *corev1.Reference) (*emptypb.Empty, error) {
	return nil, nil
}
