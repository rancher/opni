package test

import (
	"context"

	"github.com/golang/mock/gomock"
	"github.com/rancher/opni-monitoring/pkg/storage"
	mock_storage "github.com/rancher/opni-monitoring/pkg/test/mock/storage"
)

func NewTestKeyValueStoreBroker(ctrl *gomock.Controller) storage.KeyValueStoreBroker {
	mockKvStoreBroker := mock_storage.NewMockKeyValueStoreBroker(ctrl)
	kvStores := map[string]storage.KeyValueStore{}
	mockKvStoreBroker.EXPECT().
		KeyValueStore(gomock.Any()).
		DoAndReturn(func(namespace string) (storage.KeyValueStore, error) {
			if kvStore, ok := kvStores[namespace]; !ok {
				s := NewTestKeyValueStore(ctrl)
				kvStores[namespace] = s
				return s, nil
			} else {
				return kvStore, nil
			}
		}).
		AnyTimes()
	return mockKvStoreBroker
}

func NewTestKeyValueStore(ctrl *gomock.Controller) storage.KeyValueStore {
	mockKvStore := mock_storage.NewMockKeyValueStore(ctrl)
	kvs := map[string][]byte{}
	mockKvStore.EXPECT().
		Put(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string, value []byte) error {
			kvs[key] = value
			return nil
		}).
		AnyTimes()
	mockKvStore.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string) ([]byte, error) {
			v, ok := kvs[key]
			if !ok {
				return nil, storage.ErrNotFound
			}
			return v, nil
		}).
		AnyTimes()
	return mockKvStore
}
