package test

import (
	"context"

	"github.com/golang/mock/gomock"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/storage"
	mock_storage "github.com/rancher/opni-monitoring/pkg/test/mock/storage"
)

type KeyringStoreHandler = func(_ context.Context, prefix string, ref *core.Reference) (storage.KeyringStore, error)

func NewTestKeyringStoreBroker(ctrl *gomock.Controller, handler ...KeyringStoreHandler) storage.KeyringStoreBroker {
	mockKeyringStoreBroker := mock_storage.NewMockKeyringStoreBroker(ctrl)
	keyringStores := map[string]storage.KeyringStore{}
	defaultHandler := func(_ context.Context, prefix string, ref *core.Reference) (storage.KeyringStore, error) {
		if keyringStore, ok := keyringStores[prefix+ref.Id]; !ok {
			s := NewTestKeyringStore(ctrl, prefix, ref)
			keyringStores[prefix+ref.Id] = s
			return s, nil
		} else {
			return keyringStore, nil
		}
	}

	var h KeyringStoreHandler
	if len(handler) > 0 {
		h = handler[0]
	} else {
		h = defaultHandler
	}

	mockKeyringStoreBroker.EXPECT().
		KeyringStore(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, prefix string, ref *core.Reference) (storage.KeyringStore, error) {
			if prefix == "gateway-internal" {
				return defaultHandler(ctx, prefix, ref)
			}
			return h(ctx, prefix, ref)
		}).
		AnyTimes()
	return mockKeyringStoreBroker
}

func NewTestKeyringStore(ctrl *gomock.Controller, prefix string, ref *core.Reference) storage.KeyringStore {
	mockKeyringStore := mock_storage.NewMockKeyringStore(ctrl)
	keyrings := map[string]keyring.Keyring{}
	mockKeyringStore.EXPECT().
		Put(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, keyring keyring.Keyring) error {
			keyrings[prefix+ref.Id] = keyring
			return nil
		}).
		AnyTimes()
	mockKeyringStore.EXPECT().
		Get(gomock.Any()).
		DoAndReturn(func(_ context.Context) (keyring.Keyring, error) {
			keyring, ok := keyrings[prefix+ref.Id]
			if !ok {
				return nil, storage.ErrNotFound
			}
			return keyring, nil
		}).
		AnyTimes()
	return mockKeyringStore
}
