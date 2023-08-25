package system

import (
	"context"
	"reflect"

	"github.com/rancher/opni/pkg/storage"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type kvStoreServer struct {
	UnsafeKeyValueStoreServer
	store storage.KeyValueStore
}

func (s *kvStoreServer) Put(ctx context.Context, in *PutRequest) (*PutResponse, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	var revOut int64
	opts := []storage.PutOpt{
		storage.WithRevisionOut(&revOut),
	}
	if in.Revision != nil {
		opts = append(opts, storage.WithRevision(*in.Revision))
	}
	err := s.store.Put(ctx, in.Key, in.Value, opts...)
	if err != nil {
		return nil, err
	}
	return &PutResponse{
		Revision: revOut,
	}, nil
}

func (s *kvStoreServer) Get(ctx context.Context, in *GetRequest) (*GetResponse, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	var revOut int64
	opts := []storage.GetOpt{
		storage.WithRevisionOut(&revOut),
	}
	if in.Revision != nil {
		opts = append(opts, storage.WithRevision(*in.Revision))
	}
	data, err := s.store.Get(ctx, in.GetKey(), opts...)
	if err != nil {
		return nil, err
	}
	return &GetResponse{
		Value:    data,
		Revision: revOut,
	}, nil
}

func (s *kvStoreServer) Watch(in *WatchRequest, stream KeyValueStore_WatchServer) error {
	opts := []storage.WatchOpt{}
	if in.Revision != nil {
		opts = append(opts, storage.WithRevision(*in.Revision))
	}
	if in.Prefix {
		opts = append(opts, storage.WithPrefix())
	}

	ch, err := s.store.Watch(stream.Context(), in.GetKey(), opts...)
	if err != nil {
		return err
	}
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			var eventType WatchResponse_EventType
			switch event.EventType {
			case storage.WatchEventPut:
				eventType = WatchResponse_Put
			case storage.WatchEventDelete:
				eventType = WatchResponse_Delete
			}
			resp := &WatchResponse{
				EventType: eventType,
			}
			if event.Current != nil {
				resp.Current = &KeyRevision{
					Key:      event.Current.Key(),
					Value:    event.Current.Value(),
					Revision: event.Current.Revision(),
				}
				if ts := event.Current.Timestamp(); !ts.IsZero() {
					resp.Current.Timestamp = timestamppb.New(ts)
				}
			}
			if event.Previous != nil {
				resp.Previous = &KeyRevision{
					Key:      event.Previous.Key(),
					Value:    event.Previous.Value(),
					Revision: event.Previous.Revision(),
				}
				if ts := event.Previous.Timestamp(); !ts.IsZero() {
					resp.Previous.Timestamp = timestamppb.New(ts)
				}
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
		}
	}
}

func (s *kvStoreServer) Delete(ctx context.Context, in *DeleteRequest) (*DeleteResponse, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	opts := []storage.DeleteOpt{}
	if in.Revision != nil {
		opts = append(opts, storage.WithRevision(*in.Revision))
	}
	err := s.store.Delete(ctx, in.GetKey(), opts...)
	if err != nil {
		return nil, err
	}
	return &DeleteResponse{}, nil
}

func (s *kvStoreServer) ListKeys(ctx context.Context, in *ListKeysRequest) (*ListKeysResponse, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	opts := []storage.ListOpt{}
	if in.Limit != nil {
		opts = append(opts, storage.WithLimit(*in.Limit))
	}
	items, err := s.store.ListKeys(ctx, in.GetKey(), opts...)
	if err != nil {
		return nil, err
	}
	return &ListKeysResponse{
		Keys: items,
	}, nil
}

func (s *kvStoreServer) History(ctx context.Context, in *HistoryRequest) (*HistoryResponse, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	opts := []storage.HistoryOpt{}
	opts = append(opts, storage.IncludeValues(in.IncludeValues))

	items, err := s.store.History(ctx, in.GetKey(), opts...)
	if err != nil {
		return nil, err
	}
	var revisions []*KeyRevision
	for _, item := range items {
		rev := &KeyRevision{
			Key:      item.Key(),
			Revision: item.Revision(),
		}
		rev.Value = item.Value()
		if ts := item.Timestamp(); !ts.IsZero() {
			rev.Timestamp = timestamppb.New(ts)
		}
		revisions = append(revisions)
	}
	return &HistoryResponse{
		Revisions: revisions,
	}, nil
}

type kvStoreClientImpl[T proto.Message] struct {
	client KeyValueStoreClient
}

func (c *kvStoreClientImpl[T]) Put(ctx context.Context, key string, value T, opts ...storage.PutOpt) error {
	options := storage.PutOptions{}
	options.Apply(opts...)

	wire, err := proto.Marshal(value)
	if err != nil {
		return err
	}
	req := &PutRequest{
		Key:      key,
		Value:    wire,
		Revision: options.Revision,
	}
	resp, err := c.client.Put(ctx, req)
	if err != nil {
		return err
	}
	if options.RevisionOut != nil {
		*options.RevisionOut = resp.Revision
	}
	return err
}

func (c *kvStoreClientImpl[T]) Get(ctx context.Context, key string, opts ...storage.GetOpt) (T, error) {
	options := storage.GetOptions{}
	options.Apply(opts...)

	resp, err := c.client.Get(ctx, &GetRequest{
		Key:      key,
		Revision: options.Revision,
	})
	if err != nil {
		return lo.Empty[T](), err
	}
	if options.RevisionOut != nil {
		*options.RevisionOut = resp.Revision
	}

	var t T
	tType := reflect.TypeOf(t)
	rt := reflect.New(tType.Elem()).Interface().(T)
	err = proto.Unmarshal(resp.GetValue(), rt)
	if err != nil {
		return t, err
	}
	return rt, nil
}

func (c *kvStoreClientImpl[T]) Watch(ctx context.Context, key string, opts ...storage.WatchOpt) (<-chan storage.WatchEvent[storage.KeyRevision[T]], error) {
	options := storage.WatchOptions{}
	options.Apply(opts...)

	stream, err := c.client.Watch(ctx, &WatchRequest{
		Key:      key,
		Revision: options.Revision,
		Prefix:   options.Prefix,
	})
	if err != nil {
		return nil, err
	}
	ch := make(chan storage.WatchEvent[storage.KeyRevision[T]], 64)
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				return
			}
			var eventType storage.WatchEventType
			switch resp.EventType {
			case WatchResponse_Put:
				eventType = storage.WatchEventPut
			case WatchResponse_Delete:
				eventType = storage.WatchEventDelete
			}
			var current storage.KeyRevision[T]
			if resp.Current != nil {
				current = FromKeyRevisionProto[T](resp.Current)
			}
			var previous storage.KeyRevision[T]
			if resp.Previous != nil {
				previous = FromKeyRevisionProto[T](resp.Previous)
			}
			ch <- storage.WatchEvent[storage.KeyRevision[T]]{
				EventType: eventType,
				Current:   current,
				Previous:  previous,
			}
		}
	}()
	return ch, nil
}

func (c *kvStoreClientImpl[T]) Delete(ctx context.Context, key string, opts ...storage.DeleteOpt) error {
	options := storage.DeleteOptions{}
	options.Apply(opts...)

	_, err := c.client.Delete(ctx, &DeleteRequest{
		Key:      key,
		Revision: options.Revision,
	})
	return err
}

func (c *kvStoreClientImpl[T]) ListKeys(ctx context.Context, prefix string, opts ...storage.ListOpt) ([]string, error) {
	options := storage.ListKeysOptions{}
	options.Apply(opts...)

	resp, err := c.client.ListKeys(ctx, &ListKeysRequest{
		Key:   prefix,
		Limit: options.Limit,
	})
	if err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

func (c *kvStoreClientImpl[T]) History(ctx context.Context, key string, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
	options := storage.HistoryOptions{}
	options.Apply(opts...)

	resp, err := c.client.History(ctx, &HistoryRequest{
		Key:           key,
		IncludeValues: options.IncludeValues,
	})
	if err != nil {
		return nil, err
	}
	var revisions []storage.KeyRevision[T]
	for _, rev := range resp.Revisions {
		revisions = append(revisions, FromKeyRevisionProto[T](rev))
	}
	return revisions, nil
}

func NewKVStoreClient[T proto.Message](client KeyValueStoreClient) storage.KeyValueStoreT[T] {
	return &kvStoreClientImpl[T]{
		client: client,
	}
}

func FromKeyRevisionProto[T proto.Message](krProto *KeyRevision) storage.KeyRevision[T] {
	kr := &storage.KeyRevisionImpl[T]{
		K:   krProto.Key,
		Rev: krProto.Revision,
	}
	if v := krProto.Value; v != nil {
		var t T
		tType := reflect.TypeOf(t)
		rt := reflect.New(tType.Elem()).Interface().(T)
		err := proto.Unmarshal(krProto.GetValue(), rt)
		if err != nil {
			return nil
		}
		kr.V = rt
	}
	if ts := krProto.Timestamp; ts != nil {
		kr.Time = ts.AsTime()
	}
	return kr
}

func ToKeyRevisionProto[T proto.Message](kr storage.KeyRevision[T]) *KeyRevision {
	var wire []byte
	if v := kr.Value(); proto.Message(v) != nil {
		var err error
		wire, err = proto.Marshal(v)
		if err != nil {
			return nil
		}
	}
	krProto := &KeyRevision{
		Key:      kr.Key(),
		Value:    wire,
		Revision: kr.Revision(),
	}
	if ts := kr.Timestamp(); !ts.IsZero() {
		krProto.Timestamp = timestamppb.New(ts)
	}
	return krProto
}
