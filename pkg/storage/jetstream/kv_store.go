package jetstream

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/storage"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type jetstreamKeyValueStore struct {
	kv nats.KeyValue
}

func (j jetstreamKeyValueStore) Put(_ context.Context, key string, value []byte, opts ...storage.PutOpt) error {
	if err := validateKey(key); err != nil {
		return err
	}

	options := storage.PutOptions{}
	options.Apply(opts...)

	var err error
	var rev uint64
	if options.Revision != nil {
		rev, err = j.kv.Update(key, value, uint64(*options.Revision))
	} else {
		rev, err = j.kv.Put(key, value)
	}
	if err != nil {
		return jetstreamGrpcError(err)
	}
	if options.RevisionOut != nil {
		*options.RevisionOut = int64(rev)
	}
	return nil
}

func (j jetstreamKeyValueStore) Get(_ context.Context, key string, opts ...storage.GetOpt) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}

	options := storage.GetOptions{}
	options.Apply(opts...)

	var resp nats.KeyValueEntry
	var err error
	if options.Revision != nil {
		resp, err = j.kv.GetRevision(key, uint64(*options.Revision))
		if errors.Is(err, nats.ErrKeyNotFound) {
			// to match the behavior of other storage backends, in the event that
			// the requested revision is not found, we need to check if the key
			// itself exists, or if the revision itself is not found, since nats
			// doesn't distinguish between the two
			if entry, err := j.kv.Get(key); err == nil {
				// the key exists
				if entry.Revision() < uint64(*options.Revision) {
					return nil, status.Errorf(codes.OutOfRange, "revision %d is a future revision", *options.Revision)
				} else {
					return nil, status.Errorf(codes.NotFound, "revision %d not found", *options.Revision)
				}
			}
		}
	} else {
		resp, err = j.kv.Get(key)
	}
	if err != nil {
		return nil, jetstreamGrpcError(err)
	}
	if options.RevisionOut != nil {
		*options.RevisionOut = int64(resp.Revision())
	}
	return resp.Value(), nil
}

func (j jetstreamKeyValueStore) Delete(_ context.Context, key string, opts ...storage.DeleteOpt) error {
	if err := validateKey(key); err != nil {
		return err
	}

	options := storage.DeleteOptions{}
	options.Apply(opts...)

	var clientOptions []nats.DeleteOpt
	if options.Revision != nil {
		clientOptions = append(clientOptions, nats.LastRevision(uint64(*options.Revision)))
	}
	if _, err := j.kv.Get(key); err != nil {
		return jetstreamGrpcError(err)
	}
	if err := j.kv.Delete(key, clientOptions...); err != nil {
		return jetstreamGrpcError(err)
	}
	return nil
}

func (j jetstreamKeyValueStore) ListKeys(ctx context.Context, prefix string, opts ...storage.ListOpt) ([]string, error) {
	options := storage.ListKeysOptions{}
	options.Apply(opts...)

	keys, err := j.kv.Keys(nats.Context(ctx), nats.MetaOnly())
	if err != nil {
		if errors.Is(err, nats.ErrNoKeysFound) {
			return []string{}, nil
		}
		return nil, jetstreamGrpcError(err)
	}
	filtered := lo.Filter(keys, func(key string, _ int) bool {
		return strings.HasPrefix(key, prefix)
	})
	if options.Limit != nil && int64(len(filtered)) > *options.Limit {
		filtered = filtered[:*options.Limit]
	}
	return filtered, nil
}

func (j *jetstreamKeyValueStore) History(ctx context.Context, key string, opts ...storage.HistoryOpt) ([]storage.KeyRevision[[]byte], error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}

	options := storage.HistoryOptions{}
	options.Apply(opts...)

	clientOptions := []nats.WatchOpt{
		nats.Context(ctx),
	}
	if !options.IncludeValues {
		clientOptions = append(clientOptions, nats.MetaOnly())
	}

	entries, err := j.kv.History(key, clientOptions...)
	if err != nil {
		return nil, jetstreamGrpcError(err)
	}
	revs := make([]storage.KeyRevision[[]byte], 0, len(entries))
	if len(entries) > 0 && entries[len(entries)-1].Operation() == nats.KeyValueDelete {
		if options.Revision == nil || *options.Revision == int64(entries[len(entries)-1].Revision()) {
			return nil, storage.ErrNotFound
		}
	}
ENTRIES:
	for _, entry := range entries {
		switch entry.Operation() {
		case nats.KeyValuePut:
			if options.Revision != nil && entry.Revision() > uint64(*options.Revision) {
				break ENTRIES
			}
			rev := &storage.KeyRevisionImpl[[]byte]{
				K:    entry.Key(),
				Rev:  int64(entry.Revision()),
				Time: entry.Created(),
			}
			if options.IncludeValues {
				rev.V = entry.Value()
			}
			revs = append(revs, rev)
		case nats.KeyValueDelete:
			break ENTRIES
		}
	}
	return revs, nil
}

type keyOnlyRevision struct {
	key       string
	revision  uint64
	timestamp time.Time
}

func (r *keyOnlyRevision) Key() string {
	return r.key
}

func (r *keyOnlyRevision) Value() ([]byte, bool) {
	return nil, false
}

func (r *keyOnlyRevision) Revision() int64 {
	return int64(r.revision)
}

func (r *keyOnlyRevision) Timestamp() time.Time {
	return r.timestamp
}

type keyValueRevision struct {
	key       string
	value     []byte
	revision  uint64
	timestamp time.Time
}

func (r *keyValueRevision) Key() string {
	return r.key
}

func (r *keyValueRevision) Value() ([]byte, bool) {
	return r.value, true
}

func (r *keyValueRevision) Revision() int64 {
	return int64(r.revision)
}

func (r *keyValueRevision) Timestamp() time.Time {
	return r.timestamp
}

func validateKey(key string) error {
	if key == "" {
		return status.Errorf(codes.InvalidArgument, "key cannot be empty")
	}
	return nil
}
