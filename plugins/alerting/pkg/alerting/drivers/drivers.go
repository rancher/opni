package drivers

import (
	"context"

	alertmanager "github.com/rancher/opni/internal/cortex/config/alertmanager"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	v1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	alertopsv2 "github.com/rancher/opni/plugins/alerting/pkg/apis/alertops/v2"
)

type CortexAMConfiguration struct {
	*alertmanager.MultitenantAlertmanagerConfig
	*v1.StorageSpec
	*alertopsv2.ResourceLimits
}

type ClusterDriverV2 interface {
	Install(ctx context.Context, conf *CortexAMConfiguration) error
	Configure(ctx context.Context, conf *CortexAMConfiguration) error
	GetConfiguration(ctx context.Context) (*CortexAMConfiguration, error)
	Status(ctx context.Context) (*alertopsv2.InstallStatus, error)
	Uninstall(ctx context.Context) error
}

var DriversV2 = driverutil.NewDriverCache[ClusterDriverV2]()

type NoopClusterDriverV2 struct{}

var _ ClusterDriverV2 = (*NoopClusterDriverV2)(nil)

func (n NoopClusterDriverV2) Install(_ context.Context, _ *CortexAMConfiguration) error {
	return nil
}

func (n NoopClusterDriverV2) Configure(_ context.Context, _ *CortexAMConfiguration) error {
	return nil
}

func (n NoopClusterDriverV2) GetConfiguration(_ context.Context) (*CortexAMConfiguration, error) {
	return nil, nil
}

func (n NoopClusterDriverV2) Status(_ context.Context) (*alertopsv2.InstallStatus, error) {
	return nil, nil
}

func (n NoopClusterDriverV2) Uninstall(ctx context.Context) error {
	return nil
}

func (n NoopClusterDriverV2) ClientSet() {
}

type ClusterDriver interface {
	alertops.AlertingAdminServer
	// ShouldDisableNode is called during node sync for nodes which otherwise
	// have this capability enabled. If this function returns an error, the
	// node will be set to disabled instead, and the error will be logged.
	ShouldDisableNode(*corev1.Reference) error
	GetRuntimeOptions() shared.AlertingClusterOptions
}

var Drivers = driverutil.NewDriverCache[ClusterDriver]()

type NoopClusterDriver struct {
	alertops.UnimplementedAlertingAdminServer
}

func (d *NoopClusterDriver) ShouldDisableNode(*corev1.Reference) error {
	// the noop driver will never forcefully disable a node
	return nil
}

func (d *NoopClusterDriver) GetRuntimeOptions() shared.AlertingClusterOptions {
	return shared.AlertingClusterOptions{}
}

func init() {
	Drivers.Register("noop", func(ctx context.Context, opts ...driverutil.Option) (ClusterDriver, error) {
		return &NoopClusterDriver{}, nil
	})
	DriversV2.Register("noop", func(ctx context.Context, opts ...driverutil.Option) (ClusterDriverV2, error) {
		return &NoopClusterDriverV2{}, nil
	})
}
