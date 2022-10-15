package alertstorage

import (
	"context"
	"path"

	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/protobuf/proto"
)

func list[T proto.Message](ctx context.Context, kvc storage.KeyValueStoreT[T], prefix string) ([]T, error) {
	keys, err := kvc.ListKeys(ctx, prefix)
	if err != nil {
		return nil, err
	}
	items := make([]T, len(keys))
	for i, key := range keys {
		item, err := kvc.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	return items, nil
}

func listWithKeys[T proto.Message](ctx context.Context, kvc storage.KeyValueStoreT[T], prefix string) ([]string, []T, error) {
	keys, err := kvc.ListKeys(ctx, prefix)
	if err != nil {
		return nil, nil, err
	}
	items := make([]T, len(keys))
	ids := make([]string, len(keys))
	for i, key := range keys {
		item, err := kvc.Get(ctx, key)

		if err != nil {
			return nil, nil, err
		}
		items[i] = item
		ids[i] = path.Base(key)
	}
	return ids, items, nil
}
