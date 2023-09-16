package storage

type CompositeBackend struct {
	TokenStore
	ClusterStore
	RBACStore
	KeyringStoreBroker
	KeyValueStoreBroker
	LockManagerBroker
}

var _ Backend = (*CompositeBackend)(nil)
var _ LockManagerBroker = (*CompositeBackend)(nil)

func (cb *CompositeBackend) Use(store any) {
	if ts, ok := store.(TokenStore); ok {
		cb.TokenStore = ts
	}
	if cs, ok := store.(ClusterStore); ok {
		cb.ClusterStore = cs
	}
	if rb, ok := store.(RBACStore); ok {
		cb.RBACStore = rb
	}
	if ks, ok := store.(KeyringStoreBroker); ok {
		cb.KeyringStoreBroker = ks
	}
	if kv, ok := store.(KeyValueStoreBroker); ok {
		cb.KeyValueStoreBroker = kv
	}
	if lm, ok := store.(LockManagerBroker); ok {
		cb.LockManagerBroker = lm
	}
}

func (cb *CompositeBackend) IsValid() bool {
	return cb.TokenStore != nil &&
		cb.ClusterStore != nil &&
		cb.RBACStore != nil &&
		cb.KeyringStoreBroker != nil &&
		cb.KeyValueStoreBroker != nil
}
