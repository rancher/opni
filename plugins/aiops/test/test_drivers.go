package test

import (
	"context"
	"errors"

	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/aiops/pkg/features/metric_anomaly/drivers"
)

type TestEnvDashboardDriver struct {
	env *test.Environment
}

func (o *TestEnvDashboardDriver) CreateDashboard(name string, jsonData string) error {
	return errors.New("not implemented")
}

func (o *TestEnvDashboardDriver) DeleteDashboard(name string) error {
	return errors.New("not implemented")
}

func init() {
	drivers.DashboardDrivers.Register("test-environment", func(ctx context.Context, opts ...driverutil.Option) (drivers.DashboardDriver, error) {
		env := test.EnvFromContext(ctx)
		env.Logger.Info("using test environment dashboard driver")
		return &TestEnvDashboardDriver{
			env: env,
		}, nil
	})
}
