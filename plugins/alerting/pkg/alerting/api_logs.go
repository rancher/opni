package alerting

import (
	"context"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/slo/shared"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) CreateAlertLog(ctx context.Context, event *corev1.AlertLog) (*emptypb.Empty, error) {
	return nil, shared.WithUnimplementedError(
		"not implemented",
	)
}
func (p *Plugin) ListAlertLogs(ctx context.Context, req *alertingv1.ListAlertLogRequest) (*alertingv1.InformativeAlertLogList, error) {
	return nil, shared.WithUnimplementedError(
		"not implemented",
	)
}
