package alerting

import (
	"context"
	"fmt"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	apis "github.com/rancher/opni/plugins/alerting/pkg/apis/alerting"
	"google.golang.org/protobuf/types/known/emptypb"
)

// --- Log ---
func (p *Plugin) CreateAlertLog(ctx context.Context, event *apis.AlertLog) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) GetAlertLog(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) ListAlertLogs(ctx context.Context, req *apis.ListAlertLogRequest) (*apis.AlertLogList, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) UpdateAlertLog(ctx context.Context, event *apis.UpdateAlertLogRequest) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) DeleteAlertLog(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

// --- Trigger ---
func (p *Plugin) TriggerAlerts(ctx context.Context, req *apis.TriggerAlertsRequest) (*apis.TriggerAlertsResponse, error) {
	return nil, fmt.Errorf("Not implemented")
}

// --- Alert Conditions ---

func (p *Plugin) CreateAlertCondition(ctx context.Context, req *apis.AlertCondition) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*apis.AlertCondition, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) ListAlertConditions(ctx context.Context, req *apis.ListAlertConditionRequest) (*apis.AlertConditionList, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) UpdateAlertCondition(ctx context.Context, req *apis.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) PreviewAlertCondition(ctx context.Context, req *apis.PreviewAlertConditionRequest) (*apis.PreviewAlertConditionResponse, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) CreateAlertEndpoint(ctx context.Context, req *apis.AlertEndpoint) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*apis.AlertEndpoint, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *apis.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context, req *apis.ListAlertEndpointsRequest) (*apis.AlertEndpointList, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *apis.TestAlertEndpointRequest) (*apis.TestAlertEndpointResponse, error) {
	return nil, fmt.Errorf("Not implemented")
}
