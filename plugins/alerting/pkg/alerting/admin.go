package alerting

/*
TODO : implementation of the API requires some code restructuring
*/

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	alertopsv2 "github.com/rancher/opni/plugins/alerting/pkg/apis/alertops/v2"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ alertopsv2.AlertingAdminV2Server = (*Plugin)(nil)

func (p *Plugin) GetClusterConfiguration(context.Context, *emptypb.Empty) (*alertopsv2.ClusterConfiguration, error) {
	return nil, status.Error(codes.Unimplemented, "api not enabled")
}

func (p *Plugin) ConfigureCluster(context.Context, *alertopsv2.ClusterConfiguration) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "api not enabled")
}

func (p *Plugin) GetClusterStatus(context.Context, *emptypb.Empty) (*alertopsv2.InstallStatus, error) {
	return nil, status.Error(codes.Unimplemented, "api not enabled")
}

func (p *Plugin) UninstallCluster(context.Context, *alertopsv2.UninstallRequest) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "api not enabled")
}
