package system

import (
	"context"
	"reflect"

	"github.com/rancher/opni/pkg/storage"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type kvStoreServer struct {
	UnsafeKeyValueStoreServer
	store storage.KeyValueStore
}

func (s *kvStoreServer) Put(ctx context.Context, kv *KeyValue) (*emptypb.Empty, error) {
	err := s.store.Put(ctx, kv.GetKey(), kv.GetValue())
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *kvStoreServer) Get(ctx context.Context, key *Key) (*Value, error) {
	data, err := s.store.Get(ctx, key.GetKey())
	if err != nil {
		return nil, err
	}
	return &Value{
		Value: data,
	}, nil
}

func (s *kvStoreServer) Delete(ctx context.Context, key *Key) (*emptypb.Empty, error) {
	err := s.store.Delete(ctx, key.GetKey())
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *kvStoreServer) ListKeys(ctx context.Context, key *Key) (*KeyList, error) {
	items, err := s.store.ListKeys(ctx, key.GetKey())
	if err != nil {
		return nil, err
	}
	return &KeyList{
		Items: items,
	}, nil
}

type kvStoreClientImpl[T proto.Message] struct {
	client KeyValueStoreClient
}

func (c *kvStoreClientImpl[T]) Put(ctx context.Context, key string, value T) error {
	wire, err := proto.Marshal(value)
	if err != nil {
		return err
	}
	kv := &KeyValue{
		Key:   key,
		Value: wire,
	}
	_, err = c.client.Put(ctx, kv)
	return err
}

func (c *kvStoreClientImpl[T]) Get(ctx context.Context, key string) (T, error) {
	value, err := c.client.Get(ctx, &Key{
		Key: key,
	})
	if err != nil {
		return lo.Empty[T](), err
	}

	var t T
	tType := reflect.TypeOf(t)
	rt := reflect.New(tType.Elem()).Interface().(T)
	err = proto.Unmarshal(value.GetValue(), rt)
	if err != nil {
		return t, err
	}
	return rt, nil
}

func (c *kvStoreClientImpl[T]) Delete(ctx context.Context, key string) error {
	_, err := c.client.Delete(ctx, &Key{
		Key: key,
	})
	return err
}

func (c *kvStoreClientImpl[T]) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	resp, err := c.client.ListKeys(ctx, &Key{
		Key: prefix,
	})
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func NewKVStoreClient[T proto.Message](client KeyValueStoreClient) storage.KeyValueStoreT[T] {
	return &kvStoreClientImpl[T]{
		client: client,
	}
}
