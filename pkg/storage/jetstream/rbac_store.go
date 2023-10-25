package jetstream

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s *JetStreamStore) CreateRoleBinding(_ context.Context, rb *corev1.RoleBinding) error {
	data, err := protojson.Marshal(rb)
	if err != nil {
		return err
	}
	rev, err := s.kv.RoleBindings.Create(rb.Id, data)
	if errors.Is(err, nats.ErrKeyExists) {
		return storage.ErrAlreadyExists
	}
	rb.SetResourceVersion(fmt.Sprint(rev))
	return err
}

func (s *JetStreamStore) UpdateRoleBinding(ctx context.Context, ref *corev1.Reference, mutator storage.RoleBindingMutator) (*corev1.RoleBinding, error) {
	p := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(1*time.Millisecond),
		backoff.WithMaxInterval(128*time.Millisecond),
		backoff.WithMultiplier(2),
	)
	b := p.Start(ctx)
	var updateErr error
	for backoff.Continue(b) {
		rb, err := s.GetRoleBinding(ctx, ref)
		if err != nil {
			return nil, err
		}
		versionStr := rb.GetResourceVersion()
		version, err := strconv.ParseUint(versionStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("internal error: role binding has invalid resource version: %w", err)
		}
		mutator(rb)
		data, err := protojson.Marshal(rb)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal role binding: %w", err)
		}
		rev, err := s.kv.RoleBindings.Update(ref.Id, data, version)
		if err != nil {
			updateErr = err
			continue
		}
		rb.SetResourceVersion(fmt.Sprint(rev))
		return rb, nil
	}
	if updateErr != nil {
		return nil, fmt.Errorf("failed to update role binding: %w", updateErr)
	}
	return nil, fmt.Errorf("failed to update role binding: (unknown error)")
}

func (s *JetStreamStore) DeleteRoleBinding(_ context.Context, ref *corev1.Reference) error {
	if _, err := s.kv.RoleBindings.Get(ref.Id); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return storage.ErrNotFound
		}
		return err
	}
	return s.kv.RoleBindings.Delete(ref.Id)
}

func (s *JetStreamStore) GetRoleBinding(_ context.Context, ref *corev1.Reference) (*corev1.RoleBinding, error) {
	entry, err := s.kv.RoleBindings.Get(ref.Id)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) || (errors.Is(err, nats.ErrInvalidKey) && ref.GetId() == "") {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	rb := &corev1.RoleBinding{}
	if err := protojson.Unmarshal(entry.Value(), rb); err != nil {
		return nil, err
	}
	rb.SetResourceVersion(fmt.Sprint(entry.Revision()))
	return rb, nil
}

func (s *JetStreamStore) ListRoleBindings(ctx context.Context) (*corev1.RoleBindingList, error) {
	watcher, err := s.kv.RoleBindings.WatchAll(nats.IgnoreDeletes(), nats.Context(ctx))
	if err != nil {
		return nil, err
	}
	defer watcher.Stop()

	var rbs []*corev1.RoleBinding
	for entry := range watcher.Updates() {
		if entry == nil {
			break
		}

		rb := &corev1.RoleBinding{}
		if err := protojson.Unmarshal(entry.Value(), rb); err != nil {
			return nil, err
		}
		rbs = append(rbs, rb)
	}
	return &corev1.RoleBindingList{
		Items: rbs,
	}, nil
}
