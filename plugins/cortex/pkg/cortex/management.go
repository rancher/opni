package cortex

import (
	"context"

	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexops"
	"github.com/rancher/opni/plugins/cortex/pkg/drivers"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) configureCortexManagement() {
	// load default cluster drivers
	drivers.ResetClusterDrivers()
	if kcd, err := drivers.NewKubernetesClusterDriver(); err == nil {
		drivers.RegisterClusterDriver(kcd)
	} else {
		drivers.LogDriverFailure(kcd.Name(), err) // Name() is safe to call on a nil pointer
	}

	driverName := p.config.Get().Spec.Cortex.Management.ClusterDriver
	if driverName == "" {
		p.logger.Warn("no cluster driver configured, cortex-ops api will be disabled")
		return
	}

	driver, err := drivers.GetClusterDriver(driverName)
	if err != nil {
		p.logger.With(
			"driver", driverName,
			zap.Error(err),
		).Error("failed to load cluster driver, cortex-ops api will be disabled")
		return
	}

	p.clusterDriver.Store(driver)
}

func (p *Plugin) ConfigureInstall(ctx context.Context, in *cortexops.InstallConfiguration) (*emptypb.Empty, error) {
	driver := p.clusterDriver.Load()
	if driver == nil {
		return nil, status.Error(codes.Unimplemented, "api disabled")
	}

	return driver.ConfigureInstall(ctx, in)
}

func (p *Plugin) GetInstallStatus(ctx context.Context, in *emptypb.Empty) (*cortexops.InstallStatus, error) {
	driver := p.clusterDriver.Load()
	if driver == nil {
		return nil, status.Error(codes.Unimplemented, "api disabled")
	}

	return driver.GetInstallStatus(ctx, in)
}

func (p *Plugin) UninstallCluster(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
	driver := p.clusterDriver.Load()
	if driver == nil {
		return nil, status.Error(codes.Unimplemented, "api disabled")
	}

	return driver.UninstallCluster(ctx, in)
}
