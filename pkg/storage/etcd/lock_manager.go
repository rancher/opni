package etcd

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/util"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

type EtcdLockManager struct {
	lg      *zap.SugaredLogger
	options EtcdStoreOptions
	client  *clientv3.Client
}

func NewEtcdLockManager(ctx context.Context, conf *v1beta1.EtcdStorageSpec, opts ...EtcdStoreOption) (*EtcdLockManager, error) {
	options := EtcdStoreOptions{}
	options.apply(opts...)
	lg := logger.New(logger.WithLogLevel(zap.WarnLevel)).Named("etcd")
	var tlsConfig *tls.Config
	if conf.Certs != nil {
		var err error
		tlsConfig, err = util.LoadClientMTLSConfig(conf.Certs)
		if err != nil {
			return nil, fmt.Errorf("failed to load client TLS config: %w", err)
		}
	}
	clientConfig := clientv3.Config{
		Endpoints: conf.Endpoints,
		TLS:       tlsConfig,
		Context:   ctx,
		Logger:    lg.Desugar(),
	}
	cli, err := clientv3.New(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}
	lg.With(
		"endpoints", clientConfig.Endpoints,
	).Info("connecting to etcd")
	return &EtcdLockManager{
		options: options,
		lg:      lg,
		client:  cli,
	}, nil
}

var _ storage.LockManager = (*EtcdLockManager)(nil)

func (e *EtcdLockManager) Locker(key string, opts ...lock.LockOption) storage.Lock {
	options := lock.NewLockOptions(e.client.Ctx())
	options.Apply(opts...)
	return NewEtcdLock(e.client, key, options)
}
