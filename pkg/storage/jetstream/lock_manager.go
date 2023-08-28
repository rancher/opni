package jetstream

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"go.uber.org/zap"
)

var (
	GlobalLockId = 0
)

var _ storage.Lock = (*JetstreamLock)(nil)
var _ storage.LockManager = (*JetstreamLockManager)(nil)

type JetstreamLockManager struct {
	lg *zap.SugaredLogger

	ctx context.Context
	kv  nats.KeyValue

	uuid       string
	clientPool *lock.LockPool
}

func NewLockManager(ctx context.Context, conf *v1beta1.JetStreamStorageSpec) (*JetstreamLockManager, error) {
	lg := logger.New(logger.WithLogLevel(zap.WarnLevel)).Named("jetstreamLockManager")

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
	bucketName := "opni-lock-manager"
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      bucketName,
		Description: "opni store for distributed locking",
		Storage:     nats.FileStorage,
		Replicas:    1,
	})
	if err != nil {
		lg.With("bucket", bucketName, zap.Error(err)).Panic("failed to create bucket")
		return nil, err
	}

	return &JetstreamLockManager{
		lg:         logger.NewPluginLogger().Named("lock-manager"),
		ctx:        ctx,
		kv:         kv,
		clientPool: lock.NewLockPool(),
		uuid:       uuid.New().String(),
	}, nil
}

func (l *JetstreamLockManager) Locker(key string, opts ...lock.LockOption) storage.Lock {
	options := lock.NewLockOptions(l.ctx)
	options.Apply(opts...)
	return NewJetstreamLock(l.ctx, l.lg, l.kv, key, options, l.clientPool, l.uuid, lock.EX)
}
