/*
Module implements a noop alerting model, in case the alerting plugin
is not loaded
*/
package noop

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/interfaces"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

//
func NewUnavailableAlertingImplementation(version string) interfaces.GatewayAlertingImplementation {
	switch version {
	case shared.AlertingV1Alpha:
		return &AlertingNoopV1Alpha{}
	}
	return &AlertingNoopV1Alpha{}
}

type AlertingNoopV1Alpha struct{}

func (a *AlertingNoopV1Alpha) CreateAlertLog(ctx context.Context, event *corev1.AlertLog) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) GetAlertLog(ctx context.Context, ref *corev1.Reference) (*corev1.AlertLog, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) ListAlertLogs(ctx context.Context, req *alertingv1alpha.ListAlertLogRequest) (*corev1.AlertLogList, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) UpdateAlertLog(ctx context.Context, req *alertingv1alpha.UpdateAlertLogRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) DeleteAlertLog(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) TriggerAlerts(ctx context.Context, req *alertingv1alpha.TriggerAlertsRequest) (*alertingv1alpha.TriggerAlertsResponse, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) CreateAlertCondition(ctx context.Context, req *alertingv1alpha.AlertCondition) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertCondition, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) ListAlertConditions(ctx context.Context, req *alertingv1alpha.ListAlertConditionRequest) (*alertingv1alpha.AlertConditionList, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) UpdateAlertCondition(ctx context.Context, req *alertingv1alpha.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) PreviewAlertCondition(ctx context.Context, req *alertingv1alpha.PreviewAlertConditionRequest) (*alertingv1alpha.PreviewAlertConditionResponse, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) CreateAlertEndpoint(ctx context.Context, req *alertingv1alpha.AlertEndpoint) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertEndpoint, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) UpdateAlertEndpoint(ctx context.Context, req *alertingv1alpha.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) ListAlertEndpoints(ctx context.Context, req *alertingv1alpha.ListAlertEndpointsRequest) (*alertingv1alpha.AlertEndpointList, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) TestAlertEndpoint(ctx context.Context, req *alertingv1alpha.TestAlertEndpointRequest) (*alertingv1alpha.TestAlertEndpointResponse, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
