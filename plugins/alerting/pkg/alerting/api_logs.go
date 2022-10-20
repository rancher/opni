package alerting

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/slo/shared"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) CreateAlertLog(ctx context.Context, event *corev1.AlertLog) (*emptypb.Empty, error) {
	return nil, shared.WithUnimplementedError(
		"not implemented",
	)
}
func (p *Plugin) ListAlertLogs(ctx context.Context, req *alertingv1alpha.ListAlertLogRequest) (*alertingv1alpha.InformativeAlertLogList, error) {
	return nil, shared.WithUnimplementedError(
		"not implemented",
	)
}
