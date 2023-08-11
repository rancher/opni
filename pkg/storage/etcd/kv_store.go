package etcd

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"strings"

	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rancher/opni/pkg/storage"
)

type genericKeyValueStore struct {
	client *clientv3.Client
	prefix string
}

func etcdGrpcError(err error) error {
	e, ok := err.(rpctypes.EtcdError)
	if !ok {
		return err
	}
	return status.Error(e.Code(), e.Error())
}

func (s *genericKeyValueStore) Put(ctx context.Context, key string, value []byte, opts ...storage.PutOpt) error {
	options := storage.PutOptions{}
	options.Apply(opts...)

	if err := validateKey(key); err != nil {
		return err
	}
	qualifiedKey := path.Join(s.prefix, key)
	encodedValue := base64.StdEncoding.EncodeToString(value)

	var comparisons []clientv3.Cmp
	if options.Revision != nil {
		if *options.Revision > 0 {
			comparisons = []clientv3.Cmp{clientv3.Compare(clientv3.ModRevision(qualifiedKey), "=", *options.Revision)}
		} else {
			comparisons = []clientv3.Cmp{clientv3.Compare(clientv3.Version(qualifiedKey), "=", 0)}
		}
	}
	resp, err := s.client.Txn(ctx).
		If(comparisons...).
		Then(clientv3.OpPut(qualifiedKey, encodedValue)).
		Commit()
	if err != nil {
		return etcdGrpcError(err)
	}
	if !resp.Succeeded {
		return fmt.Errorf("%w: revision mismatch", storage.ErrConflict)
	}
	if options.RevisionOut != nil {
		*options.RevisionOut = resp.Header.Revision
	}
	return nil
}

func (s *genericKeyValueStore) Get(ctx context.Context, key string, opts ...storage.GetOpt) ([]byte, error) {
	options := storage.GetOptions{}
	options.Apply(opts...)

	if err := validateKey(key); err != nil {
		return nil, err
	}
	clientOptions := []clientv3.OpOption{}
	if options.Revision != nil {
		clientOptions = append(clientOptions, clientv3.WithRev(*options.Revision))
	}
	resp, err := s.client.Get(ctx, path.Join(s.prefix, key), clientOptions...)
	if err != nil {
		return nil, etcdGrpcError(err)
	}
	if len(resp.Kvs) == 0 {
		return nil, storage.ErrNotFound
	}
	if options.RevisionOut != nil {
		*options.RevisionOut = resp.Kvs[0].ModRevision
	}
	return base64.StdEncoding.DecodeString(string(resp.Kvs[0].Value))
}

func (s *genericKeyValueStore) Delete(ctx context.Context, key string, opts ...storage.DeleteOpt) error {
	options := storage.DeleteOptions{}
	options.Apply(opts...)

	if err := validateKey(key); err != nil {
		return err
	}
	qualifiedKey := path.Join(s.prefix, key)
	if options.Revision != nil {
		resp, err := s.client.Txn(ctx).
			If(clientv3.Compare(clientv3.ModRevision(qualifiedKey), "=", *options.Revision)).
			Then(clientv3.OpDelete(qualifiedKey)).
			Commit()
		if err != nil {
			return etcdGrpcError(err)
		}
		if !resp.Succeeded {
			return fmt.Errorf("%w: revision mismatch", storage.ErrConflict)
		}
		return nil
	}

	resp, err := s.client.Delete(ctx, qualifiedKey)
	if err != nil {
		return etcdGrpcError(err)
	}
	if resp.Deleted == 0 {
		return storage.ErrNotFound
	}

	return nil
}

func (s *genericKeyValueStore) ListKeys(ctx context.Context, prefix string, opts ...storage.ListOpt) ([]string, error) {
	options := storage.ListKeysOptions{}
	options.Apply(opts...)

	clientOptions := []clientv3.OpOption{
		clientv3.WithPrefix(),
		clientv3.WithKeysOnly(),
	}
	if options.Limit != nil {
		clientOptions = append(clientOptions, clientv3.WithLimit(*options.Limit))
	}
	resp, err := s.client.Get(ctx, path.Join(s.prefix, prefix), clientOptions...)
	if err != nil {
		return nil, etcdGrpcError(err)
	}
	keys := make([]string, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		keys[i] = strings.TrimPrefix(string(kv.Key), s.prefix+"/")
	}
	return keys, nil
}

func (s *genericKeyValueStore) History(ctx context.Context, key string, opts ...storage.HistoryOpt) ([]storage.KeyRevision[[]byte], error) {
	options := storage.HistoryOptions{}
	options.Apply(opts...)

	if err := validateKey(key); err != nil {
		return nil, err
	}
	clientOptions := []clientv3.OpOption{clientv3.WithLimit(1)}
	if !options.IncludeValues {
		clientOptions = append(clientOptions, clientv3.WithKeysOnly())
	}
	if options.Revision != nil {
		clientOptions = append(clientOptions, clientv3.WithRev(*options.Revision))
	}
	last, err := s.client.Get(ctx, path.Join(s.prefix, key), clientOptions...)
	if err != nil {
		return nil, etcdGrpcError(err)
	}
	if len(last.Kvs) == 0 {
		return nil, storage.ErrNotFound
	}
	createRev := last.Kvs[0].CreateRevision
	latestModRev := last.Kvs[0].ModRevision

	clientOptions = []clientv3.OpOption{}
	if !options.IncludeValues {
		clientOptions = append(clientOptions, clientv3.WithKeysOnly())
	}

	clientOptions = append(clientOptions,
		clientv3.WithRev(createRev),
	)
	revs := []storage.KeyRevision[[]byte]{}
	watchCtx, ca := context.WithCancel(ctx)
	wc := s.client.Watch(watchCtx, path.Join(s.prefix, key), clientOptions...)
	defer ca()
WATCH:
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case resp, ok := <-wc:
			if !ok {
				break WATCH
			}
			if err := resp.Err(); err != nil {
				return nil, etcdGrpcError(err)
			}
			for _, ev := range resp.Events {
				if ev.Type == clientv3.EventTypePut {
					entry := &storage.KeyRevisionImpl[[]byte]{
						K:   key,
						Rev: ev.Kv.ModRevision,
					}
					if options.IncludeValues {
						value, err := base64.StdEncoding.DecodeString(string(ev.Kv.Value))
						if err != nil {
							return nil, err
						}
						entry.V = value
					}
					revs = append(revs, entry)
					if entry.Rev == latestModRev {
						break WATCH
					}
				}
			}
		}
	}
	return revs, nil
}

func validateKey(key string) error {
	// etcd will check keys, but we need to check if the key is empty ourselves
	// since we always prepend a prefix to the key
	if key == "" {
		return status.Errorf(codes.InvalidArgument, "key cannot be empty")
	}
	return nil
}
