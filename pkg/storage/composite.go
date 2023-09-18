package storage

type CompositeBackend struct {
	TokenStore
	ClusterStore
	RBACStore
	KeyringStoreBroker
	KeyValueStoreBroker
	LockManagerBroker
}

// Close implements Backend.
func (cb CompositeBackend) Close() {
	// call Close on all unique stores once each. some stores may have the
	// same underlying object.
	uniqueStores := map[any]struct{}{}
	if cb.TokenStore != nil {
		uniqueStores[cb.TokenStore] = struct{}{}
	}
	if cb.ClusterStore != nil {
		uniqueStores[cb.ClusterStore] = struct{}{}
	}
	if cb.RBACStore != nil {
		uniqueStores[cb.RBACStore] = struct{}{}
	}
	if cb.KeyringStoreBroker != nil {
		uniqueStores[cb.KeyringStoreBroker] = struct{}{}
	}
	if cb.KeyValueStoreBroker != nil {
		uniqueStores[cb.KeyValueStoreBroker] = struct{}{}
	}
	if cb.LockManagerBroker != nil {
		uniqueStores[cb.LockManagerBroker] = struct{}{}
	}
	for store := range uniqueStores {
		if closer, ok := store.(interface{ Close() }); ok {
			closer.Close()
		}
	}
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
