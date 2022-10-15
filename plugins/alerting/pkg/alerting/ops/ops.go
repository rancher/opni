package ops

import (
	"context"
	"time"

	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AlertingOpsNode struct {
	AlertingOpsNodeOptions
	ClusterDriver future.Future[drivers.ClusterDriver]
	alertops.UnsafeAlertingAdminServer
	alertops.UnsafeDynamicAlertingServer
}

type AlertingOpsNodeOptions struct {
	timeout time.Duration
}

type AlertingOpsNodeOption func(*AlertingOpsNodeOptions)

func (a *AlertingOpsNodeOptions) apply(opts ...AlertingOpsNodeOption) {
	for _, opt := range opts {
		opt(a)
	}
}

var _ alertops.AlertingAdminServer = (*AlertingOpsNode)(nil)

func NewAlertingOpsNode(clusterDriver future.Future[drivers.ClusterDriver], opts ...AlertingOpsNodeOption) *AlertingOpsNode {
	options := AlertingOpsNodeOptions{
		timeout: 60 * time.Second,
	}
	options.apply(opts...)

	return &AlertingOpsNode{
		AlertingOpsNodeOptions: options,
		ClusterDriver:          clusterDriver,
	}
}

func (a *AlertingOpsNode) GetClusterConfiguration(ctx context.Context, _ *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.GetClusterConfiguration(ctx, &emptypb.Empty{})

}

func (a *AlertingOpsNode) ConfigureCluster(ctx context.Context, conf *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.ConfigureCluster(ctx, conf)
}

func (a *AlertingOpsNode) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.InstallStatus, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.GetClusterStatus(ctx, &emptypb.Empty{})
}

func (a *AlertingOpsNode) UninstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.UninstallCluster(ctx, &emptypb.Empty{})
}

func (a *AlertingOpsNode) InstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.InstallCluster(ctx, &emptypb.Empty{})
}

func (a *AlertingOpsNode) Fetch(ctx context.Context, _ *emptypb.Empty) (*alertops.AlertingConfig, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.Fetch(ctx, &emptypb.Empty{})
}

func (a *AlertingOpsNode) GetStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.DynamicStatus, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.GetStatus(ctx, &emptypb.Empty{})
}

func (a *AlertingOpsNode) Update(ctx context.Context, config *alertops.AlertingConfig) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.Update(ctx, config)
}

func (a *AlertingOpsNode) Reload(ctx context.Context, info *alertops.ReloadInfo) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.Reload(ctx, info)
}
