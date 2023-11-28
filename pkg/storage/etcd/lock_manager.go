package etcd

import (
	"log/slog"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdLockManager struct {
	client *clientv3.Client
	prefix string

	lg *slog.Logger
}

func NewEtcdLockManager(
	client *clientv3.Client,
	prefix string,
	lg *slog.Logger,
) *EtcdLockManager {
	lm := &EtcdLockManager{
		client: client,
		prefix: prefix,
		lg:     lg,
	}
	return lm
}

// !! Cannot reuse *concurrency.Session across multiple locks since it will break liveliness guarantee A
// locks will share their sessions and therefore keepalives will be sent for all locks, not just a specific lock.
// In the current implementation sessions are forcibly orphaned when the non-blocking call to unlock is
// made so we cannot re-use sessions in that case either -- since the session  will be orphaned for all locks
// if the session is re-used.
func (e *EtcdLockManager) NewLock(key string, opts ...lock.LockOption) storage.Lock {
	options := lock.DefaultLockOptions()
	options.Apply(opts...)

	return NewEtcdLock(
		e.lg,
		e.client,
		e.prefix,
		key,
		options,
	)
}
