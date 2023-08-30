package jetstream

import (
	"context"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"go.uber.org/zap"
)

var DefaultLeaserExpireDuration = 5 * time.Second

type JetStreamLockManager struct {
	ctx context.Context
	kv  nats.KeyValue
	lg  *zap.SugaredLogger
}

func (j *JetStreamLockManager) Locker(key string, opts ...lock.LockOption) storage.Lock {
	options := lock.DefaultLockOptions(j.ctx)
	options.Apply(opts...)
	return NewLock(
		j.ctx,
		j.kv,
		key,
		options,
		j.lg,
	)
}

func NewLockManager(ctx context.Context, conf *v1beta1.JetStreamStorageSpec, opts ...JetStreamStoreOption) (*JetStreamLockManager, error) {
	lg := logger.New(logger.WithLogLevel(zap.DebugLevel)).Named("jetstream")

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
				zap.Error(err),
			).Warn("disconnected from jetstream")
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			lg.With(
				"server", c.ConnectedAddr(),
				"id", c.ConnectedServerId(),
				"name", c.ConnectedServerName(),
				"version", c.ConnectedServerVersion(),
			).Info("reconnected to jetstream")
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
			lg.With("rtt", rtt).Info("nats server connection is healthy")
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

	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      "lock-leaser",
		Description: "TODO",
		Storage:     nats.FileStorage,
		MaxBytes:    1024 * 1024 * 1024,
		TTL:         DefaultLeaserExpireDuration,
	})
	if err != nil {
		return nil, err
	}

	return &JetStreamLockManager{
		ctx: ctx,
		kv:  kv,
		lg:  lg,
	}, nil
}
