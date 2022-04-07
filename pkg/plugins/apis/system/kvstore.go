package system

import (
	"context"

	"github.com/rancher/opni-monitoring/pkg/storage"
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

func (s *kvStoreServer) ListKeys(ctx context.Context, key *Key) (*KeyList, error) {
	items, err := s.store.ListKeys(ctx, key.GetKey())
	if err != nil {
		return nil, err
	}
	return &KeyList{
		Items: items,
	}, nil
}

type KVStoreClient interface {
	Put(key string, value proto.Message) error
	Get(key string, out proto.Message) error
	ListKeys(prefix string) ([]string, error)
}

type kvStoreClientImpl struct {
	ctx    context.Context
	client KeyValueStoreClient
}

func (c *kvStoreClientImpl) Put(key string, value proto.Message) error {
	wire, err := proto.Marshal(value)
	if err != nil {
		return err
	}
	kv := &KeyValue{
		Key:   key,
		Value: wire,
	}
	_, err = c.client.Put(c.ctx, kv)
	return err
}

func (c *kvStoreClientImpl) Get(key string, out proto.Message) error {
	value, err := c.client.Get(c.ctx, &Key{
		Key: key,
	})
	if err != nil {
		return err
	}

	return proto.Unmarshal(value.GetValue(), out)
}

func (c *kvStoreClientImpl) ListKeys(prefix string) ([]string, error) {
	resp, err := c.client.ListKeys(c.ctx, &Key{
		Key: prefix,
	})
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}
