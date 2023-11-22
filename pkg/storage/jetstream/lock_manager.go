package jetstream

import (
	"context"
	"log/slog"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
)

// Requires jetstream 2.9+
type LockManager struct {
	ctx context.Context
	js  nats.JetStreamContext

	lg *slog.Logger
}

// Requires jetstream 2.9+
func NewJetStreamLockManager(ctx context.Context, conf *v1beta1.JetStreamStorageSpec, lg *slog.Logger) (*LockManager, error) {
	nkeyOpt, err := nats.NkeyOptionFromSeed(conf.NkeySeedPath)
	if err != nil {
		return nil, err
	}

	nc, err := nats.Connect(conf.Endpoint,
		nkeyOpt,
		nats.MaxReconnects(-1),
		nats.RetryOnFailedConnect(true),
		nats.DisconnectErrHandler(func(c *nats.Conn, err error) {
			lg.With(
				logger.Err(err),
			).Warn("disconnected from jetstream")
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			lg.Info(
				"reconnected to jetstream", "server", c.ConnectedAddr(),
				"id", c.ConnectedServerId(),
				"name", c.ConnectedServerName(),
				"version", c.ConnectedServerVersion())

		}),
	)
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		nc.Close()
	}()

	ctrl := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(10*time.Millisecond),
		backoff.WithMaxInterval(10*time.Millisecond<<9),
		backoff.WithMultiplier(2.0),
	).Start(ctx)
	for {
		if rtt, err := nc.RTT(); err == nil {
			lg.Info("nats server connection is healthy", "rtt", rtt)
			break
		}
		select {
		case <-ctrl.Done():
			return nil, ctx.Err()
		case <-ctrl.Next():
		}
	}

	js, err := nc.JetStream(nats.Context(ctx))
	if err != nil {
		return nil, err
	}

	return &LockManager{
		js:  js,
		ctx: ctx,
		lg:  lg,
	}, nil
}

var _ storage.LockManager = (*LockManager)(nil)

func (l *LockManager) Locker(key string, opts ...lock.LockOption) storage.Lock {
	options := lock.DefaultLockOptions()
	options.Apply(opts...)
	return NewLock(l.js, key, l.lg, options)
}
