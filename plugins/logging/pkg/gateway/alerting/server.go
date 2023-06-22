package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/logging/pkg/apis/alerting"
	loggingutil "github.com/rancher/opni/plugins/logging/pkg/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AlertingManagementServer struct {
	alerting.UnsafeAlertManagementServer
	alerting.UnsafeNotificationManagementServer
	alerting.UnsafeMonitorManagementServer

	*loggingutil.AsyncOpensearchClient
}

var (
	_ alerting.AlertManagementServer        = (*AlertingManagementServer)(nil)
	_ alerting.NotificationManagementServer = (*AlertingManagementServer)(nil)
	_ alerting.MonitorManagementServer      = (*AlertingManagementServer)(nil)
)

func NewAlertingManagementServer() *AlertingManagementServer {
	return &AlertingManagementServer{
		AsyncOpensearchClient: loggingutil.NewAsyncOpensearchClient(),
	}
}

func (a *AlertingManagementServer) CreateMonitor(ctx context.Context, req *alerting.Monitor) (*emptypb.Empty, error) {
	resp, err := a.Alerting.CreateMonitor(ctx, bytes.NewReader(req.GetSpec()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("open search failed to create the monitor %s", resp.Status))
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertingManagementServer) GetMonitor(ctx context.Context, ref *corev1.Reference) (*alerting.Monitor, error) {
	resp, err := a.Alerting.GetMonitor(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("opensearch failed to get the monitor : %s", resp.Status))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to decode response body into []byte %s", err))
	}
	return &alerting.Monitor{
		MonitorId:   ref.GetId(),
		MonitorType: "", // tbd
		Spec:        data,
	}, nil
}

func (a *AlertingManagementServer) UpdateMonitor(ctx context.Context, req *alerting.Monitor) (*emptypb.Empty, error) {
	resp, err := a.Alerting.UpdateMonitor(ctx, req.GetMonitorId(), bytes.NewReader(req.GetSpec()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("opensearch failed to update the monitor : %s", resp.Status))
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertingManagementServer) DeleteMonitor(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	resp, err := a.Alerting.DeleteMonitor(ctx, ref.GetId())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("opensearch failed to delete the monitor : %s", resp.Status))
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertingManagementServer) CreateNotification(ctx context.Context, req *alerting.Channel) (*emptypb.Empty, error) {
	resp, err := a.Alerting.CreateMonitor(ctx, bytes.NewReader(req.GetSpec()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("open search failed to create the destination %s", resp.Status))
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertingManagementServer) GetNotification(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	resp, err := a.Alerting.GetNotification(ctx, ref.GetId())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("opensearch failed to get the destination : %s", resp.Status))
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertingManagementServer) ListNotifications(ctx context.Context, _ *emptypb.Empty) (*alerting.ChannelList, error) {
	resp, err := a.Alerting.ListNotifications(ctx)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("opensearch failed to list the destinations : %s", resp.Status))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to decode response body into []byte %s", err))
	}
	return &alerting.ChannelList{
		List: data,
	}, nil
}

func (a *AlertingManagementServer) UpdateNotification(ctx context.Context, req *alerting.Channel) (*emptypb.Empty, error) {
	resp, err := a.Alerting.UpdateNotification(ctx, req.GetChannelId(), bytes.NewReader(req.GetSpec()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("opensearch failed to update the destination : %s", resp.Status))
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertingManagementServer) DeleteDestination(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	resp, err := a.Alerting.DeleteNotification(ctx, ref.GetId())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("opensearch failed to delete the destination : %s", resp.Status))
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertingManagementServer) ListAlerts(ctx context.Context, _ *emptypb.Empty) (*alerting.ListAlertsResponse, error) {
	resp, err := a.Alerting.ListAlerts(ctx)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("opensearch failed to list the alerts : %s", resp.Status))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to decode response body into []byte %s", err))
	}
	return &alerting.ListAlertsResponse{
		Alerts: data,
	}, nil
}

func (a *AlertingManagementServer) AcknowledgeAlert(ctx context.Context, req *alerting.AcknowledgeAlertRequest) (*emptypb.Empty, error) {
	type A struct {
		Alerts []string `json:"alerts"`
	}
	var b bytes.Buffer
	err := json.NewEncoder(&b).Encode(A{
		Alerts: req.GetAlertIds(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to encode acknowledge request body %s", err))
	}
	resp, err := a.Alerting.AcknowledgeAlert(ctx, req.GetMonitorId(), bytes.NewReader(b.Bytes()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, status.Error(codes.Internal, fmt.Sprintf("opensearch failed to acknowledge the alert : %s", resp.Status))
	}
	return &emptypb.Empty{}, nil
}
