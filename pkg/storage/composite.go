package storage

type CompositeBackend struct {
	TokenStore
	ClusterStore
	RoleBindingStore
	KeyringStoreBroker
	KeyValueStoreBroker
}

var _ Backend = (*CompositeBackend)(nil)

func (cb *CompositeBackend) Use(store any) {
	if ts, ok := store.(TokenStore); ok {
		cb.TokenStore = ts
	}
	if cs, ok := store.(ClusterStore); ok {
		cb.ClusterStore = cs
	}
	if rb, ok := store.(RoleBindingStore); ok {
		cb.RoleBindingStore = rb
	}
	if ks, ok := store.(KeyringStoreBroker); ok {
		cb.KeyringStoreBroker = ks
	}
	if kv, ok := store.(KeyValueStoreBroker); ok {
		cb.KeyValueStoreBroker = kv
	}
}

func (cb *CompositeBackend) IsValid() bool {
	return cb.TokenStore != nil &&
		cb.ClusterStore != nil &&
		cb.RoleBindingStore != nil &&
		cb.KeyringStoreBroker != nil &&
		cb.KeyValueStoreBroker != nil
}
