/*
Module implements a noop alerting model, in case the alerting plugin
is not loaded
*/
package noop

import (
	"context"

	"github.com/rancher/opni/pkg/alerting"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

//
func NewUnavailableAlertingImplementation(version string) alerting.Provider {
	switch version {
	case shared.AlertingV1Alpha:
		return &AlertingNoopV1Alpha{}
	}
	return &AlertingNoopV1Alpha{}
}

type AlertingNoopV1Alpha struct{}

func (a *AlertingNoopV1Alpha) CreateAlertLog(ctx context.Context, event *corev1.AlertLog, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) GetAlertLog(ctx context.Context, ref *corev1.Reference, opts ...grpc.CallOption) (*corev1.AlertLog, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) ListAlertLogs(ctx context.Context, req *alertingv1alpha.ListAlertLogRequest, opts ...grpc.CallOption) (*alertingv1alpha.InformativeAlertLogList, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) UpdateAlertLog(ctx context.Context, req *alertingv1alpha.UpdateAlertLogRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) DeleteAlertLog(ctx context.Context, ref *corev1.Reference, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) TriggerAlerts(ctx context.Context, req *alertingv1alpha.TriggerAlertsRequest, opts ...grpc.CallOption) (*alertingv1alpha.TriggerAlertsResponse, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) CreateAlertCondition(ctx context.Context, req *alertingv1alpha.AlertCondition, opts ...grpc.CallOption) (*corev1.Reference, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) GetAlertCondition(ctx context.Context, ref *corev1.Reference, opts ...grpc.CallOption) (*alertingv1alpha.AlertCondition, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) ListAlertConditions(ctx context.Context, req *alertingv1alpha.ListAlertConditionRequest, opts ...grpc.CallOption) (*alertingv1alpha.AlertConditionList, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) UpdateAlertCondition(ctx context.Context, req *alertingv1alpha.UpdateAlertConditionRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) PreviewAlertCondition(ctx context.Context, req *alertingv1alpha.PreviewAlertConditionRequest, opts ...grpc.CallOption) (*alertingv1alpha.PreviewAlertConditionResponse, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) CreateAlertEndpoint(ctx context.Context, req *alertingv1alpha.AlertEndpoint, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference, opts ...grpc.CallOption) (*alertingv1alpha.AlertEndpoint, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) UpdateAlertEndpoint(ctx context.Context, req *alertingv1alpha.UpdateAlertEndpointRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) ListAlertEndpoints(ctx context.Context, req *alertingv1alpha.ListAlertEndpointsRequest, opts ...grpc.CallOption) (*alertingv1alpha.AlertEndpointList, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
func (a *AlertingNoopV1Alpha) TestAlertEndpoint(ctx context.Context, req *alertingv1alpha.TestAlertEndpointRequest, opts ...grpc.CallOption) (*alertingv1alpha.TestAlertEndpointResponse, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}

func (a *AlertingNoopV1Alpha) GetImplementationFromEndpoint(ctx context.Context, ref *corev1.Reference, opts ...grpc.CallOption) (*alertingv1alpha.EndpointImplementation, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}

func (a *AlertingNoopV1Alpha) CreateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}

func (a *AlertingNoopV1Alpha) UpdateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}

func (a *AlertingNoopV1Alpha) DeleteEndpointImplementation(ctx context.Context, ref *corev1.Reference, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplementedNOOP
}
