package alarms

import (
	"context"
	"fmt"

	"github.com/rancher/opni/pkg/alerting/drivers/cortex"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
)

const (
	metadataCleanUpAlarm = "opni.io/alarm-cleanup"
)

func (p *AlarmServerComponent) teardownCondition(
	ctx context.Context,
	req *alertingv1.AlertCondition,
	id string,
	cleanup bool,
) (retErr error) {
	defer func() {
		if cleanup && retErr == nil {
			condStorage, err := p.conditionStorage.GetContext(ctx)
			if err != nil {
				retErr = err
				return
			}
			if err := condStorage.Group(req.GroupId).Delete(ctx, id); err != nil {
				retErr = err
			}
		}
	}()
	if req.GetMetadata() != nil && req.GetMetadata()[metadataInactiveAlarm] != "" {
		return nil
	}
	if alertingv1.IsInternalCondition(req) {
		incidentStorage, err := p.incidentStorage.GetContext(ctx)
		if err != nil {
			return err
		}
		stateStorage, err := p.stateStorage.GetContext(ctx)
		if err != nil {
			return err
		}
		p.runner.RemoveConfigListener(id)
		if err := incidentStorage.Delete(ctx, id); err != nil {
			retErr = err
		}
		if err := stateStorage.Delete(ctx, id); err != nil {
			retErr = err
		}
		return
	}
	if alertingv1.IsMetricsCondition(req) {
		if r, _ := extractClusterMd(req.AlertType); r != nil {
			cortexAdminClient, err := p.adminClient.GetContext(ctx)
			if err != nil {
				return err
			}
			_, err = cortexAdminClient.DeleteRule(ctx, &cortexadmin.DeleteRuleRequest{
				ClusterId: r.Id,
				Namespace: shared.OpniAlertingCortexNamespace,
				GroupName: cortex.RuleIdFromUuid(id),
			})
			retErr = err
			return
		} else {
			retErr = fmt.Errorf("failed to extract clusterId from metrics condition %s", req.GetId())
			return
		}
	}
	return shared.AlertingErrNotImplemented
}

func extractClusterMd(t *alertingv1.AlertTypeDetails) (*corev1.Reference, alertingv1.IndexableMetric) {
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
