package test

import (
	"context"

	"github.com/golang/mock/gomock"
	"github.com/rancher/opni-monitoring/pkg/storage"
)

func NewTestStorageBackend(ctx context.Context, ctrl *gomock.Controller) storage.Backend {
	return &storage.CompositeBackend{
		TokenStore:          NewTestTokenStore(ctx, ctrl),
		ClusterStore:        NewTestClusterStore(ctrl),
		RBACStore:           NewTestRBACStore(ctrl),
		KeyringStoreBroker:  NewTestKeyringStoreBroker(ctrl),
		KeyValueStoreBroker: NewTestKeyValueStoreBroker(ctrl),
	}
}
