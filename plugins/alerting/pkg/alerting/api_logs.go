package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

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
