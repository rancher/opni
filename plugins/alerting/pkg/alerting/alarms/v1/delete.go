package alarms

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/drivers/cortex"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"
)

func (p *AlarmServerComponent) deleteCondition(ctx context.Context, _ *zap.SugaredLogger, req *alertingv1.AlertCondition, id string) error {
	if r := req.GetAlertType().GetSystem(); r != nil {
		p.runner.RemoveConfigListener(id)
		p.incidentStorage.Get().Delete(ctx, id)
		p.stateStorage.Get().Delete(ctx, id)
		return nil
	}
	if r := req.AlertType.GetDownstreamCapability(); r != nil {
		p.runner.RemoveConfigListener(id)
		p.incidentStorage.Get().Delete(ctx, id)
		p.stateStorage.Get().Delete(ctx, id)
		return nil
	}
	if r := req.AlertType.GetMonitoringBackend(); r != nil {
		p.runner.RemoveConfigListener(id)
		p.incidentStorage.Get().Delete(ctx, id)
		p.stateStorage.Get().Delete(ctx, id)
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
