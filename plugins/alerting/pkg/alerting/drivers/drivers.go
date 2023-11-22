package drivers

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
)

type ClusterDriver interface {
	alertops.AlertingAdminServer
	// ShouldDisableNode is called during node sync for nodes which otherwise
	// have this capability enabled. If this function returns an error, the
	// node will be set to disabled instead, and the error will be logged.
	ShouldDisableNode(*corev1.Reference) error
	GetDefaultReceiver() *config.WebhookConfig
}

var Drivers = driverutil.NewCache[ClusterDriver]()

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

func (d *NoopClusterDriver) GetDefaultReceiver() *config.WebhookConfig {
	return &config.WebhookConfig{}
}

func init() {
	Drivers.Register("noop", func(ctx context.Context, opts ...driverutil.Option) (ClusterDriver, error) {
		return &NoopClusterDriver{}, nil
	})
}
