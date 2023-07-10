package jetstream

import (
	"context"
	"errors"

	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s *JetStreamStore) CreateRole(_ context.Context, role *corev1.Role) error {
	data, err := protojson.Marshal(role)
	if err != nil {
		return err
	}
	_, err = s.kv.Roles.Create(role.Id, data)
	if errors.Is(err, nats.ErrKeyExists) {
		return storage.ErrAlreadyExists
	}
	return err
}

func (s *JetStreamStore) UpdateRole(_ context.Context, role *corev1.Role) error {
	data, err := protojson.Marshal(role)
	if err != nil {
		return err
	}
	if _, err := s.kv.Roles.Get(role.Id); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return storage.ErrNotFound
		}
		return err
	}
	_, err = s.kv.Roles.Put(role.Id, data)
	return err
}

func (s *JetStreamStore) DeleteRole(_ context.Context, ref *corev1.Reference) error {
	if _, err := s.kv.Roles.Get(ref.Id); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return storage.ErrNotFound
		}
		return err
	}
	return s.kv.Roles.Delete(ref.Id)
}

func (s *JetStreamStore) GetRole(_ context.Context, ref *corev1.Reference) (*corev1.Role, error) {
	entry, err := s.kv.Roles.Get(ref.Id)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) || (errors.Is(err, nats.ErrInvalidKey) && ref.GetId() == "") {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	role := &corev1.Role{}
	if err := protojson.Unmarshal(entry.Value(), role); err != nil {
		return nil, err
	}
	return role, nil
}

func (s *JetStreamStore) CreateRoleBinding(_ context.Context, rb *corev1.RoleBinding) error {
	data, err := protojson.Marshal(rb)
	if err != nil {
		return err
	}
	_, err = s.kv.RoleBindings.Create(rb.Id, data)
	if errors.Is(err, nats.ErrKeyExists) {
		return storage.ErrAlreadyExists
	}
	return err
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

func (s *JetStreamStore) GetRoleBinding(ctx context.Context, ref *corev1.Reference) (*corev1.RoleBinding, error) {
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
	if err := storage.ApplyRoleBindingTaints(ctx, s, rb); err != nil {
		return nil, err
	}
	return rb, nil
}

func (s *JetStreamStore) ListRoles(ctx context.Context) (*corev1.RoleList, error) {
	watcher, err := s.kv.Roles.WatchAll(nats.IgnoreDeletes(), nats.Context(ctx))
	if err != nil {
		return nil, err
	}
	defer watcher.Stop()

	var roles []*corev1.Role
	for entry := range watcher.Updates() {
		if entry == nil {
			break
		}

		role := &corev1.Role{}
		if err := protojson.Unmarshal(entry.Value(), role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return &corev1.RoleList{
		Items: roles,
	}, nil
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
