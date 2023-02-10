package alerting

import (
	"context"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/slo/shared"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) CreateAlertLog(_ context.Context, _ *corev1.AlertLog) (*emptypb.Empty, error) {
	return nil, shared.WithUnimplementedError(
		"not implemented",
	)
}
func (p *Plugin) ListAlertLogs(_ context.Context, _ *alertingv1.ListAlertLogRequest) (*alertingv1.InformativeAlertLogList, error) {
	return nil, shared.WithUnimplementedError(
		"not implemented",
	)
}
