package machinery

import (
	"context"
	"errors"
	"fmt"

	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/storage"
)

func ConfigureStorageBackend(ctx context.Context, cfg *v1beta1.StorageSpec) (storage.Backend, error) {
	storageBackend := storage.CompositeBackend{}
	builder := storage.GetStoreBuilder(cfg.Type)
	if builder == nil {
		return nil, fmt.Errorf("unknown storage backend %s", cfg.Type)
	}
	var store any
	var err error
	switch cfg.Type {
	case v1beta1.StorageTypeEtcd:
		options := cfg.Etcd
		if options == nil {
			return nil, errors.New("etcd storage options are not set")
		}
		store, err = builder(ctx, cfg.Etcd, "gateway")
	case v1beta1.StorageTypeCRDs:
		options := cfg.CustomResources
		crdOpts := []any{}
		if options != nil {
			crdOpts = append(crdOpts, options.Namespace)
		}
		store, err = builder(crdOpts...)
	case v1beta1.StorageTypeJetStream:
		options := cfg.JetStream
		if options == nil {
			return nil, errors.New("jetstream storage options are not set")
		}
		store, err = builder(ctx, options)
	default:
		return nil, errors.New("unknown storage type")
	}
	if err != nil {
		return nil, err
	}
	storageBackend.Use(store)
	return storageBackend, nil
}

func ConfigureStorageBackendV1(ctx context.Context, cfg *configv1.StorageSpec) (storage.Backend, error) {
	storageBackend := storage.CompositeBackend{}
	builder := storage.GetStoreBuilder(cfg.GetBackend().String())
	if builder == nil {
		return nil, fmt.Errorf("unknown storage backend %s", cfg.GetBackend().String())
	}
	var store any
	var err error
	switch cfg.GetBackend() {
	case configv1.StorageBackend_Etcd:
		options := cfg.Etcd
		if options == nil {
			return nil, errors.New("etcd storage options are not set")
		}
		store, err = builder(ctx, cfg.Etcd, "gateway")
	case configv1.StorageBackend_JetStream:
		options := cfg.JetStream
		if options == nil {
			return nil, errors.New("jetstream storage options are not set")
		}
		store, err = builder(ctx, options)
	default:
		return nil, errors.New("unknown storage type")
	}
	if err != nil {
		return nil, err
	}
	storageBackend.Use(store)
	return storageBackend, nil
}
