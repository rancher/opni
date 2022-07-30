package alerting

import (
	"context"

	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// alerting interface to be injected into the gateway
//
// Should at least encapsulate all alerting plugin implementations
type Provider interface {
	CreateAlertLog(ctx context.Context, event *corev1.AlertLog) (*emptypb.Empty, error)
	GetAlertLog(ctx context.Context, ref *corev1.Reference) (*corev1.AlertLog, error)
	ListAlertLogs(ctx context.Context, req *alertingv1alpha.ListAlertLogRequest) (*alertingv1alpha.InformativeAlertLogList, error)
	UpdateAlertLog(ctx context.Context, req *alertingv1alpha.UpdateAlertLogRequest) (*emptypb.Empty, error)
	DeleteAlertLog(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error)
	TriggerAlerts(ctx context.Context, req *alertingv1alpha.TriggerAlertsRequest) (*alertingv1alpha.TriggerAlertsResponse, error)
	CreateAlertCondition(ctx context.Context, req *alertingv1alpha.AlertCondition) (*emptypb.Empty, error)
	GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertCondition, error)
	ListAlertConditions(ctx context.Context, req *alertingv1alpha.ListAlertConditionRequest) (*alertingv1alpha.AlertConditionList, error)
	UpdateAlertCondition(ctx context.Context, req *alertingv1alpha.UpdateAlertConditionRequest) (*emptypb.Empty, error)
	DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error)
	PreviewAlertCondition(ctx context.Context, req *alertingv1alpha.PreviewAlertConditionRequest) (*alertingv1alpha.PreviewAlertConditionResponse, error)
	CreateAlertEndpoint(ctx context.Context, req *alertingv1alpha.AlertEndpoint) (*emptypb.Empty, error)
	GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertEndpoint, error)
	UpdateAlertEndpoint(ctx context.Context, req *alertingv1alpha.UpdateAlertEndpointRequest) (*emptypb.Empty, error)
	ListAlertEndpoints(ctx context.Context, req *alertingv1alpha.ListAlertEndpointsRequest) (*alertingv1alpha.AlertEndpointList, error)
	DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error)
	TestAlertEndpoint(ctx context.Context, req *alertingv1alpha.TestAlertEndpointRequest) (*alertingv1alpha.TestAlertEndpointResponse, error)
	GetImplementationFromEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.EndpointImplementation, error)
	CreateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation) (*emptypb.Empty, error)
	UpdateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation) (*emptypb.Empty, error)
	DeleteEndpointImplementation(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error)
}
