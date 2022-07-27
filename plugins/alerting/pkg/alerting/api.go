package alerting

import (
	"context"
	"github.com/rancher/opni/pkg/plugins/apis/system"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func list[T proto.Message](kvc system.KVStoreClient[T], prefix string) ([]T, error) {
	keys, err := kvc.ListKeys(prefix)
	if err != nil {
		return nil, err
	}
	items := make([]T, len(keys))
	for i, key := range keys {
		item, err := kvc.Get(key)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	return items, nil
}

// --- Log ---
func (p *Plugin) CreateAlertLog(ctx context.Context, event *corev1.AlertLog) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) GetAlertLog(ctx context.Context, ref *corev1.Reference) (*corev1.AlertLog, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) ListAlertLogs(ctx context.Context, req *alertingv1alpha.ListAlertLogRequest) (*corev1.AlertLogList, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) UpdateAlertLog(ctx context.Context, event *alertingv1alpha.UpdateAlertLogRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertLog(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

// --- Trigger ---
func (p *Plugin) TriggerAlerts(ctx context.Context, req *alertingv1alpha.TriggerAlertsRequest) (*alertingv1alpha.TriggerAlertsResponse, error) {
	return nil, shared.AlertingErrNotImplemented
}

// --- Alert Conditions ---

func (p *Plugin) CreateAlertCondition(ctx context.Context, req *alertingv1alpha.AlertCondition) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertCondition, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) ListAlertConditions(ctx context.Context, req *alertingv1alpha.ListAlertConditionRequest) (*alertingv1alpha.AlertConditionList, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) UpdateAlertCondition(ctx context.Context, req *alertingv1alpha.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) PreviewAlertCondition(ctx context.Context, req *alertingv1alpha.PreviewAlertConditionRequest) (*alertingv1alpha.PreviewAlertConditionResponse, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) CreateAlertEndpoint(ctx context.Context, req *alertingv1alpha.AlertEndpoint) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertEndpoint, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *alertingv1alpha.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context, req *alertingv1alpha.ListAlertEndpointsRequest) (*alertingv1alpha.AlertEndpointList, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *alertingv1alpha.TestAlertEndpointRequest) (*alertingv1alpha.TestAlertEndpointResponse, error) {
	return nil, shared.AlertingErrNotImplemented
}
