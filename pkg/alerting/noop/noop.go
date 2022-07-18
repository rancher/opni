/*
Module implements a noop alerting model, in case the alerting plugin
is not loaded
*/
package noop

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/interfaces"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	apis "github.com/rancher/opni/plugins/alerting/pkg/apis/alerting"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ interfaces.GatewayAlertingImplementation = &AlertingNoop{}

type AlertingNoop struct{}

func (a *AlertingNoop) CreateAlertLog(ctx context.Context, event *corev1.AlertLog) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) GetAlertLog(ctx context.Context, ref *corev1.Reference) (*corev1.AlertLog, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) ListAlertLogs(ctx context.Context, req *apis.ListAlertLogRequest) (*corev1.AlertLogList, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) UpdateAlertLog(ctx context.Context, req *apis.UpdateAlertLogRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) DeleteAlertLog(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) TriggerAlerts(ctx context.Context, req *apis.TriggerAlertsRequest) (*apis.TriggerAlertsResponse, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) CreateAlertCondition(ctx context.Context, req *apis.AlertCondition) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*apis.AlertCondition, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) ListAlertConditions(ctx context.Context, req *apis.ListAlertConditionRequest) (*apis.AlertConditionList, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) UpdateAlertCondition(ctx context.Context, req *apis.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) PreviewAlertCondition(ctx context.Context, req *apis.PreviewAlertConditionRequest) (*apis.PreviewAlertConditionResponse, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) CreateAlertEndpoint(ctx context.Context, req *apis.AlertEndpoint) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*apis.AlertEndpoint, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) UpdateAlertEndpoint(ctx context.Context, req *apis.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) ListAlertEndpoints(ctx context.Context, req *apis.ListAlertEndpointsRequest) (*apis.AlertEndpointList, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoop) TestAlertEndpoint(ctx context.Context, req *apis.TestAlertEndpointRequest) (*apis.TestAlertEndpointResponse, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
