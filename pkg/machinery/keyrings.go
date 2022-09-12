package machinery

import (
	"context"
	"fmt"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/crds"
	"github.com/rancher/opni/pkg/storage/etcd"
)

func BuildKeyringStoreBroker(ctx context.Context, conf v1beta1.StorageSpec) (storage.KeyringStoreBroker, error) {
	switch conf.Type {
	case v1beta1.StorageTypeEtcd:
		return etcd.NewEtcdStore(ctx, conf.Etcd), nil
	case v1beta1.StorageTypeCRDs:
		return crds.NewCRDStore(), nil
	default:
		return nil, fmt.Errorf("unknown storage type: %s", conf.Type)
	}
}
