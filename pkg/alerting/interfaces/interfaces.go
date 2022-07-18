package interfaces

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	apis "github.com/rancher/opni/plugins/alerting/pkg/apis/alerting"
	"google.golang.org/protobuf/types/known/emptypb"
)

// alerting interface to be injected into the gateway
//
// Should at least encapsulate all alerting plugin implementations
type GatewayAlertingImplementation interface {
	CreateAlertLog(ctx context.Context, event *corev1.AlertLog) (*emptypb.Empty, error)
	GetAlertLog(ctx context.Context, ref *corev1.Reference) (*corev1.AlertLog, error)
	ListAlertLogs(ctx context.Context, req *apis.ListAlertLogRequest) (*corev1.AlertLogList, error)
	UpdateAlertLog(ctx context.Context, req *apis.UpdateAlertLogRequest) (*emptypb.Empty, error)
	DeleteAlertLog(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error)
	TriggerAlerts(ctx context.Context, req *apis.TriggerAlertsRequest) (*apis.TriggerAlertsResponse, error)
	CreateAlertCondition(ctx context.Context, req *apis.AlertCondition) (*emptypb.Empty, error)
	GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*apis.AlertCondition, error)
	ListAlertConditions(ctx context.Context, req *apis.ListAlertConditionRequest) (*apis.AlertConditionList, error)
	UpdateAlertCondition(ctx context.Context, req *apis.UpdateAlertConditionRequest) (*emptypb.Empty, error)
	DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error)
	PreviewAlertCondition(ctx context.Context, req *apis.PreviewAlertConditionRequest) (*apis.PreviewAlertConditionResponse, error)
	CreateAlertEndpoint(ctx context.Context, req *apis.AlertEndpoint) (*emptypb.Empty, error)
	GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*apis.AlertEndpoint, error)
	UpdateAlertEndpoint(ctx context.Context, req *apis.UpdateAlertEndpointRequest) (*emptypb.Empty, error)
	ListAlertEndpoints(ctx context.Context, req *apis.ListAlertEndpointsRequest) (*apis.AlertEndpointList, error)
	DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error)
	TestAlertEndpoint(ctx context.Context, req *apis.TestAlertEndpointRequest) (*apis.TestAlertEndpointResponse, error)
}
