package management

import (
	"fmt"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (m *Server) CreateAlertEvent(ctx context.Context, event *managementv1.AlertEvent) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (m *Server) GetAlertEvent(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (m *Server) ListAlertEvents(ctx context.Context, req *managementv1.ListAlertEventRequest) (*managementv1.AlertEventList, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (m *Server) UpdateAlertEvent(ctx context.Context, event *managementv1.UpdateAlertEventRequest) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (m *Server) DeleteAlertEvent(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, fmt.Errorf("Not implemented")
}
