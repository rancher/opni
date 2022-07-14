package alerting

import (
	"context"
	"fmt"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) CreateAlertEvent(ctx context.Context, event *managementv1.AlertEvent) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) GetAlertEvent(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) ListAlertEvents(ctx context.Context, req *managementv1.ListAlertEventRequest) (*managementv1.AlertEventList, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) UpdateAlertEvent(ctx context.Context, event *managementv1.UpdateAlertEventRequest) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (p *Plugin) DeleteAlertEvent(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}
