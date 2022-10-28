package machinery

import (
	"context"
	"errors"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/crds"
	"github.com/rancher/opni/pkg/storage/etcd"
	"github.com/rancher/opni/pkg/storage/jetstream"
)

func ConfigureStorageBackend(ctx context.Context, cfg *v1beta1.StorageSpec) (storage.Backend, error) {
	storageBackend := storage.CompositeBackend{}
	switch cfg.Type {
	case v1beta1.StorageTypeEtcd:
		options := cfg.Etcd
		if options == nil {
			return nil, errors.New("etcd storage options are not set")
		} else {
			store := etcd.NewEtcdStore(ctx, cfg.Etcd,
				etcd.WithPrefix("gateway"),
			)
			storageBackend.Use(store)
		}
	case v1beta1.StorageTypeCRDs:
		options := cfg.CustomResources
		crdOpts := []crds.CRDStoreOption{}
		if options != nil {
			crdOpts = append(crdOpts, crds.WithNamespace(options.Namespace))
		}
		crdStore := crds.NewCRDStore(crdOpts...)
		storageBackend.Use(crdStore)
	case v1beta1.StorageTypeJetStream:
		store, err := jetstream.NewJetStreamStore(ctx, cfg.JetStream)
		if err != nil {
			return nil, err
		}
		storageBackend.Use(store)
	default:
		return nil, errors.New("unknown storage type")
	}
	return storageBackend, nil
}
