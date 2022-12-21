package ops

import (
	"context"
	"time"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Manages all dynamic backend configurations
// that must interact with & modify the runtime cluster
type AlertingOpsNode struct {
	alertops.UnsafeAlertingAdminServer
	alertops.UnsafeConfigReconcilerServer

	ctx context.Context
	AlertingOpsNodeOptions
	syncPusher chan *alertops.SyncRequest

	ClusterDriver    future.Future[drivers.ClusterDriver]
	storageClientSet future.Future[storage.AlertingClientSet]
}

var _ alertops.AlertingAdminServer = (*AlertingOpsNode)(nil)
var _ alertops.ConfigReconcilerServer = (*AlertingOpsNode)(nil)

type AlertingOpsNodeOptions struct {
	driverTimeout  time.Duration
	storageTimeout time.Duration
	logger         *zap.SugaredLogger
}

type AlertingOpsNodeOption func(*AlertingOpsNodeOptions)

func (a *AlertingOpsNodeOptions) apply(opts ...AlertingOpsNodeOption) {
	for _, opt := range opts {
		opt(a)
	}
}

var _ alertops.AlertingAdminServer = (*AlertingOpsNode)(nil)

func NewAlertingOpsNode(
	ctx context.Context,
	clusterDriver future.Future[drivers.ClusterDriver],
	storageClientSet future.Future[storage.AlertingClientSet],
	opts ...AlertingOpsNodeOption) *AlertingOpsNode {
	options := AlertingOpsNodeOptions{
		driverTimeout:  60 * time.Second,
		storageTimeout: 5 * time.Second,
	}
	options.apply(opts...)
	if options.logger == nil {
		options.logger = logger.NewPluginLogger().Named("alerting-ops")
	}

	a := &AlertingOpsNode{
		ctx:                    ctx,
		AlertingOpsNodeOptions: options,
		ClusterDriver:          clusterDriver,
		storageClientSet:       storageClientSet,
		syncPusher:             make(chan *alertops.SyncRequest),
	}
	go a.runPeriodicSync(ctx)
	return a
}

func (a *AlertingOpsNode) GetClusterConfiguration(ctx context.Context, _ *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.driverTimeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.GetClusterConfiguration(ctx, &emptypb.Empty{})

}

func (a *AlertingOpsNode) ConfigureCluster(ctx context.Context, conf *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.driverTimeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.ConfigureCluster(ctx, conf)
}

func (a *AlertingOpsNode) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.InstallStatus, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.driverTimeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.GetClusterStatus(ctx, &emptypb.Empty{})
}

func (a *AlertingOpsNode) UninstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.driverTimeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.UninstallCluster(ctx, &emptypb.Empty{})
}

func (a *AlertingOpsNode) InstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.driverTimeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.InstallCluster(ctx, &emptypb.Empty{})
}

func (a *AlertingOpsNode) GetRuntimeOptions(ctx context.Context) (shared.NewAlertingOptions, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, a.driverTimeout)
	defer cancel()
	driver, err := a.ClusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return shared.NewAlertingOptions{}, err
	}
	return driver.GetRuntimeOptions()
}

func (a *AlertingOpsNode) GetAvailableEndpoint(ctx context.Context, options shared.NewAlertingOptions) (string, error) {
	var availableEndpoint string
	status, err := a.GetClusterConfiguration(ctx, &emptypb.Empty{})
	if err != nil {
		return "", err
	}
	if status.NumReplicas == 1 { // exactly one that is the controller
		availableEndpoint = options.GetControllerEndpoint()
	} else {
		availableEndpoint = options.GetWorkerEndpoint()
	}
	return availableEndpoint, nil
}
