package health

import (
	"context"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
)

func acquireAlertingNatsConnection(ctx context.Context) (nats.JetStreamContext, error) {
	var (
		nc  *nats.Conn
		err error
	)
	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(0),
		backoffv2.WithMinInterval(5*time.Second),
		backoffv2.WithMaxInterval(1*time.Minute),
		backoffv2.WithMultiplier(1.1),
	)
	b := retrier.Start(ctx)
	for backoffv2.Continue(b) {
		nc, err = newNatsConnection()
		if err == nil {
			break
		}
		// log
	}
	mgr, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, err
	}
	if alertingStream, _ := mgr.StreamInfo(shared.AgentDisconnectStream); alertingStream == nil {
		_, err = mgr.AddStream(&nats.StreamConfig{
			Name:      shared.AgentDisconnectStream,
			Retention: nats.LimitsPolicy,
			MaxAge:    1 * time.Hour,
		})
		if err != nil {
			return nil, err
		}
	}

	return mgr, nil
}

// until we have a better way to do this
func newNatsConnection() (*nats.Conn, error) {
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
				// TODO log
			},
		),
	)
}
