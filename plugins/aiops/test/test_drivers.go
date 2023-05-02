package test

import (
	"context"
	"errors"
	"os"
	"path"

	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/aiops/pkg/features/metric_anomaly/drivers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type TestEnvDashboardDriver struct {
	env *test.Environment
}

func (o *TestEnvDashboardDriver) CreateDashboard(name string, jsonData string) error {
	path := path.Join(o.env.GetTempDirectory(), "grafana", "dashboards", name+".json")
	if _, err := os.Stat(path); err == nil {
		return status.Errorf(codes.AlreadyExists, "dashboard %s already exists", name)
	}
	if err := os.WriteFile(path, []byte(jsonData), 0644); err != nil {
		return status.Errorf(codes.Internal, "failed to create dashboard %s: %v", name, err)
	}
	return nil
}

func (o *TestEnvDashboardDriver) DeleteDashboard(name string) error {
	path := path.Join(o.env.GetTempDirectory(), "grafana", "dashboards", name+".json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return status.Errorf(codes.NotFound, "dashboard %s not found", name)
	}
	if err := os.Remove(path); err != nil {
		return status.Errorf(codes.Internal, "failed to delete dashboard %s: %v", name, err)
	}
	return nil
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
