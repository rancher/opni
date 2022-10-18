package alerting

import (
	"context"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"go.uber.org/zap"
)

func (p *Plugin) configureAlertManagerConfiguration(pluginCtx context.Context, opts ...drivers.AlertingManagerDriverOption) {
	// load default cluster drivers
	drivers.ResetClusterDrivers()
	if kcd, err := drivers.NewAlertingManagerDriver(opts...); err == nil {
		drivers.RegisterClusterDriver(kcd)
	} else {
		drivers.LogClusterDriverFailure(kcd.Name(), err) // Name() is safe to call on a nil pointer
	}

	name := "alerting-mananger"
	driver, err := drivers.GetClusterDriver(name)
	if err != nil {
		p.Logger.With(
			"driver", name,
			zap.Error(err),
		).Error("failed to load cluster driver, using fallback no-op driver")
		if os.Getenv(shared.LocalBackendEnvToggle) != "" {
			driver = drivers.NewLocalManager(
				drivers.WithLocalManagerLogger(p.Logger),
				drivers.WithLocalManagerContext(pluginCtx),
			)

		} else {
			driver = &drivers.NoopClusterDriver{}
		}
	}
	p.opsNode.ClusterDriver.Set(driver)
}

func (p *Plugin) newNatsConnection() (*nats.Conn, error) {
	natsURL := os.Getenv("NATS_SERVER_URL")
	natsSeedPath := os.Getenv("NKEY_SEED_FILENAME")

	opt, err := nats.NkeyOptionFromSeed(natsSeedPath)
	if err != nil {
		return nil, err
	}

	retryBackoff := backoff.NewExponentialBackOff()
	return nats.Connect(
		natsURL,
		opt,
		nats.MaxReconnects(-1),
		nats.CustomReconnectDelay(
			func(i int) time.Duration {
				if i == 1 {
					retryBackoff.Reset()
				}
				return retryBackoff.NextBackOff()
			},
		),
		nats.DisconnectErrHandler(
			func(nc *nats.Conn, err error) {
				p.Logger.With(
					"err", err,
				).Warn("nats disconnected")
			},
		),
	)
}
