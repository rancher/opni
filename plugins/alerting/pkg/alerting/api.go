package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	apis "github.com/rancher/opni/plugins/alerting/pkg/apis/alerting"
	"google.golang.org/protobuf/types/known/emptypb"
)

// --- Log ---
func (p *Plugin) CreateAlertLog(ctx context.Context, event *corev1.AlertLog) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) GetAlertLog(ctx context.Context, ref *corev1.Reference) (*corev1.AlertLog, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) ListAlertLogs(ctx context.Context, req *apis.ListAlertLogRequest) (*corev1.AlertLogList, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) UpdateAlertLog(ctx context.Context, event *apis.UpdateAlertLogRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertLog(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

// --- Trigger ---
func (p *Plugin) TriggerAlerts(ctx context.Context, req *apis.TriggerAlertsRequest) (*apis.TriggerAlertsResponse, error) {
	return nil, shared.AlertingErrNotImplemented
}

// --- Alert Conditions ---

func (p *Plugin) CreateAlertCondition(ctx context.Context, req *apis.AlertCondition) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*apis.AlertCondition, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) ListAlertConditions(ctx context.Context, req *apis.ListAlertConditionRequest) (*apis.AlertConditionList, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) UpdateAlertCondition(ctx context.Context, req *apis.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) PreviewAlertCondition(ctx context.Context, req *apis.PreviewAlertConditionRequest) (*apis.PreviewAlertConditionResponse, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) CreateAlertEndpoint(ctx context.Context, req *apis.AlertEndpoint) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*apis.AlertEndpoint, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *apis.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context, req *apis.ListAlertEndpointsRequest) (*apis.AlertEndpointList, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *apis.TestAlertEndpointRequest) (*apis.TestAlertEndpointResponse, error) {
	return nil, shared.AlertingErrNotImplemented
}
