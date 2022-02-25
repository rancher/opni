package test

import (
	"context"

	"github.com/golang/mock/gomock"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/storage"
	mock_storage "github.com/rancher/opni-monitoring/pkg/test/mock/storage"
)

func NewTestKeyringStoreBroker(ctrl *gomock.Controller) storage.KeyringStoreBroker {
	mockKeyringStoreBroker := mock_storage.NewMockKeyringStoreBroker(ctrl)
	keyringStores := map[string]storage.KeyringStore{}
	mockKeyringStoreBroker.EXPECT().
		KeyringStore(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, prefix string, ref *core.Reference) (storage.KeyringStore, error) {
			if keyringStore, ok := keyringStores[prefix+ref.Id]; !ok {
				s := NewTestKeyringStore(ctrl, prefix, ref)
				keyringStores[prefix+ref.Id] = s
				return s, nil
			} else {
				return keyringStore, nil
			}
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
