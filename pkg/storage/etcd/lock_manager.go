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
	lg *slog.Logger,
	prefix string,
) (*EtcdLockManager, error) {
	lm := &EtcdLockManager{
		client: client,
		prefix: prefix,
		lg:     lg,
	}
	return lm, nil
}

// !! Cannot reuse *concurrency.Session across multiple locks since it will break liveliness guarantee A
// locks will share their sessions and therefore keepalives will be sent for all locks, not just a specific lock.
// In the current implementation session's are forcibly orphaned when the non-blocking call to unlock is
// made so we cannot re-use sessions in that case either -- since the session  will be orphaned for all locks
// if the session is re-used.
func (lm *EtcdLockManager) Locker(key string, opts ...lock.LockOption) storage.Lock {
	options := lock.DefaultLockOptions()
	options.Apply(opts...)

	return NewEtcdLock(
		lm.lg,
		lm.client,
		lm.prefix,
		key,
		options,
	)
}
