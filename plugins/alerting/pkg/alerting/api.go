package alerting

import (
	"context"
	"fmt"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	apis "github.com/rancher/opni/plugins/alerting/pkg/apis/alerting"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) CreateAlertEvent(ctx context.Context, event *apis.AlertEvent) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) GetAlertEvent(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) ListAlertEvents(ctx context.Context, req *apis.ListAlertEventRequest) (*apis.AlertEventList, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) UpdateAlertEvent(ctx context.Context, event *apis.UpdateAlertEventRequest) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) DeleteAlertEvent(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}
