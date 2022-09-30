//go:build noetcd

package etcd

import (
	"context"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/storage"
)

func WithPrefix(prefix string) any {
	return nil
}

func NewEtcdStore(context.Context, *v1beta1.EtcdStorageSpec, ...any) storage.Backend {
	panic("etcd support was disabled at build time")
}
