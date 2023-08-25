package mock_storage

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	sync2 "github.com/kralicky/gpkg/sync"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/samber/lo"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

type storageErrorKey struct{}

func InjectStorageError(ctx context.Context, err error) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "mock-storage-error", err.Error())
}

func InjectedStorageError(ctx context.Context) (error, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, false
	}
	errs := md.Get("mock-storage-error")
	if len(errs) == 0 {
		return nil, false
	}
	return errors.New(errs[0]), true
}

func NewTestClusterStore(ctrl *gomock.Controller) storage.ClusterStore {
	mockClusterStore := NewMockClusterStore(ctrl)

	clusters := map[string]*v1.Cluster{}
	mu := sync.Mutex{}

	mockClusterStore.EXPECT().
		CreateCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, cluster *v1.Cluster) error {
			mu.Lock()
			defer mu.Unlock()
			if err, ok := InjectedStorageError(ctx); ok {
				return err
			}
			clusters[cluster.Id] = cluster
			return nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		DeleteCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *v1.Reference) error {
			mu.Lock()
			defer mu.Unlock()
			if err, ok := InjectedStorageError(ctx); ok {
				return err
			}
			if _, ok := clusters[ref.Id]; !ok {
				return storage.ErrNotFound
			}
			delete(clusters, ref.Id)
			return nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		ListClusters(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, matchLabels *v1.LabelSelector, matchOptions v1.MatchOptions) (*v1.ClusterList, error) {
			mu.Lock()
			defer mu.Unlock()
			if err, ok := InjectedStorageError(ctx); ok {
				return nil, err
			}
			clusterList := &v1.ClusterList{}
			selectorPredicate := storage.NewSelectorPredicate[*v1.Cluster](&v1.ClusterSelector{
				LabelSelector: matchLabels,
				MatchOptions:  matchOptions,
			})
			for _, cluster := range clusters {
				if selectorPredicate(cluster) {
					clusterList.Items = append(clusterList.Items, cluster)
				}
			}
			return clusterList, nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		GetCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *v1.Reference) (*v1.Cluster, error) {
			mu.Lock()
			defer mu.Unlock()
			if err, ok := InjectedStorageError(ctx); ok {
				return nil, err
			}
			if _, ok := clusters[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			return clusters[ref.Id], nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		UpdateCluster(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *v1.Reference, mutator storage.MutatorFunc[*v1.Cluster]) (*v1.Cluster, error) {
			mu.Lock()
			defer mu.Unlock()
			if err, ok := InjectedStorageError(ctx); ok {
				return nil, err
			}
			if _, ok := clusters[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			cluster := clusters[ref.Id]
			cloned := proto.Clone(cluster).(*v1.Cluster)
			mutator(cloned)
			if _, ok := clusters[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			clusters[ref.Id] = cloned
			return cloned, nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		WatchCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			ctx context.Context,
			cluster *v1.Cluster,
		) (<-chan storage.WatchEvent[*v1.Cluster], error) {
			if err, ok := InjectedStorageError(ctx); ok {
				return nil, err
			}
			t := time.NewTicker(time.Millisecond * 100)
			eventC := make(chan storage.WatchEvent[*v1.Cluster], 10)
			found := false
			var currentState *v1.Cluster
			mu.Lock()
			for _, observedCluster := range clusters {
				if observedCluster.GetId() == cluster.GetId() {
					currentState = observedCluster
					found = true
					if observedCluster.GetResourceVersion() != cluster.GetResourceVersion() {
						eventC <- storage.WatchEvent[*v1.Cluster]{
							EventType: storage.WatchEventPut,
							Current:   observedCluster,
							Previous:  cluster,
						}
					}
					break

				}
			}
			mu.Unlock()
			if !found {
				eventC <- storage.WatchEvent[*v1.Cluster]{
					EventType: storage.WatchEventDelete,
					Current:   nil,
					Previous:  cluster,
				}
				currentState = nil
			}

			go func() {
				defer close(eventC)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						mu.Lock()
						for _, observedCluster := range clusters {
							if observedCluster.GetId() == cluster.GetId() {
								if currentState == nil {
									eventC <- storage.WatchEvent[*v1.Cluster]{
										EventType: storage.WatchEventPut,
										Current:   observedCluster,
										Previous:  nil,
									}
								} else if observedCluster.GetResourceVersion() != currentState.GetResourceVersion() {
									eventC <- storage.WatchEvent[*v1.Cluster]{
										EventType: storage.WatchEventPut,
										Current:   observedCluster,
										Previous:  currentState,
									}
								}
								currentState = observedCluster
								break
							}
						}
						mu.Unlock()
					}
				}
			}()
			return eventC, nil
		}).AnyTimes()

	mockClusterStore.EXPECT().
		WatchClusters(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, knownClusters []*v1.Cluster) (<-chan storage.WatchEvent[*v1.Cluster], error) {
			if err, ok := InjectedStorageError(ctx); ok {
				return nil, err
			}
			t := time.NewTicker(1 * time.Millisecond * 100)

			knownClusterMap := make(map[string]*v1.Cluster, len(knownClusters))
			for _, cluster := range knownClusters {
				knownClusterMap[cluster.GetId()] = cluster
			}
			initialEvents := []storage.WatchEvent[*v1.Cluster]{}
			mu.Lock()
			for _, cluster := range clusters {
				if knownCluster, ok := knownClusterMap[cluster.GetId()]; !ok {
					initialEvents = append(initialEvents, storage.WatchEvent[*v1.Cluster]{
						EventType: storage.WatchEventPut,
						Current:   cluster,
						Previous:  nil,
					})
				} else if knownCluster.GetResourceVersion() != cluster.GetResourceVersion() {
					initialEvents = append(initialEvents, storage.WatchEvent[*v1.Cluster]{
						EventType: storage.WatchEventPut,
						Current:   cluster,
						Previous:  knownCluster,
					})
				}
			}
			mu.Unlock()
			bufSize := 100
			for len(initialEvents) > bufSize {
				bufSize *= 2
			}
			eventC := make(chan storage.WatchEvent[*v1.Cluster], bufSize)

			for _, event := range initialEvents {
				eventC <- event
			}

			go func() {
				defer close(eventC)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						observedClusters := map[string]*v1.Cluster{}
						mu.Lock()
						for _, newCl := range clusters {
							observedClusters[newCl.GetId()] = newCl
						}
						mu.Unlock()
						//create
						for _, newCl := range observedClusters {
							if _, ok := knownClusterMap[newCl.GetId()]; !ok {
								eventC <- storage.WatchEvent[*v1.Cluster]{
									EventType: storage.WatchEventPut,
									Current:   newCl,
									Previous:  nil,
								}
								knownClusterMap[newCl.GetId()] = newCl
							}
						}
						//delete or update
						for _, oldCl := range knownClusterMap {
							oldCopyCluster := oldCl //capture in closure
							if knownCluster, ok := observedClusters[oldCl.GetId()]; !ok {
								eventC <- storage.WatchEvent[*v1.Cluster]{
									EventType: storage.WatchEventDelete,
									Current:   nil,
									Previous:  oldCopyCluster,
								}
								delete(knownClusterMap, oldCl.GetId())
							} else if knownCluster.GetResourceVersion() != oldCl.GetResourceVersion() {
								eventC <- storage.WatchEvent[*v1.Cluster]{
									EventType: storage.WatchEventPut,
									Current:   knownCluster,
									Previous:  oldCopyCluster,
								}
								knownClusterMap[knownCluster.GetId()] = knownCluster
							}
						}
					}
				}
			}()
			return eventC, nil
		}).AnyTimes()
	return mockClusterStore
}

type KeyringStoreHandler = func(prefix string, ref *v1.Reference) storage.KeyringStore

func NewTestKeyringStoreBroker(ctrl *gomock.Controller, handler ...KeyringStoreHandler) storage.KeyringStoreBroker {
	mockKeyringStoreBroker := NewMockKeyringStoreBroker(ctrl)
	keyringStores := sync2.Map[string, storage.KeyringStore]{}
	defaultHandler := func(prefix string, ref *v1.Reference) storage.KeyringStore {
		if store, ok := keyringStores.Load(prefix + ref.Id); ok {
			return store
		} else {
			newStore := NewTestKeyringStore(ctrl, prefix, ref)
			keyringStores.Store(prefix+ref.Id, newStore)
			return newStore
		}
	}

	var h KeyringStoreHandler
	if len(handler) > 0 {
		h = handler[0]
	} else {
		h = defaultHandler
	}

	mockKeyringStoreBroker.EXPECT().
		KeyringStore(gomock.Any(), gomock.Any()).
		DoAndReturn(func(prefix string, ref *v1.Reference) storage.KeyringStore {
			if prefix == "gateway-internal" {
				return defaultHandler(prefix, ref)
			}
			if krs := h(prefix, ref); krs != nil {
				return krs
			}
			return defaultHandler(prefix, ref)
		}).
		AnyTimes()
	return mockKeyringStoreBroker
}

type errContextKey string

var (
	errContextValue                 errContextKey = "error"
	ErrContext_TestKeyringStore_Get               = context.WithValue(context.Background(), errContextValue, "error")
)

func NewTestKeyringStore(ctrl *gomock.Controller, prefix string, ref *v1.Reference) storage.KeyringStore {
	mockKeyringStore := NewMockKeyringStore(ctrl)
	mu := sync.Mutex{}
	keyrings := map[string]keyring.Keyring{}
	mockKeyringStore.EXPECT().
		Put(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, keyring keyring.Keyring) error {
			mu.Lock()
			defer mu.Unlock()
			keyrings[prefix+ref.Id] = keyring
			return nil
		}).
		AnyTimes()
	mockKeyringStore.EXPECT().
		Get(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (keyring.Keyring, error) {
			if ctx == ErrContext_TestKeyringStore_Get {
				return nil, errors.New("expected error")
			}
			mu.Lock()
			defer mu.Unlock()
			keyring, ok := keyrings[prefix+ref.Id]
			if !ok {
				return nil, storage.ErrNotFound
			}
			return keyring, nil
		}).
		AnyTimes()

	mockKeyringStore.EXPECT().
		Delete(gomock.Any()).
		DoAndReturn(func(_ context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := keyrings[prefix+ref.Id]; !ok {
				return storage.ErrNotFound
			}
			delete(keyrings, prefix+ref.Id)
			return nil
		}).
		AnyTimes()

	return mockKeyringStore
}

func NewTestKeyValueStoreBroker(ctrl *gomock.Controller) storage.KeyValueStoreBroker {
	mockKvStoreBroker := NewMockKeyValueStoreBroker(ctrl)
	kvStores := map[string]storage.KeyValueStore{}
	mockKvStoreBroker.EXPECT().
		KeyValueStore(gomock.Any()).
		DoAndReturn(func(namespace string) (storage.KeyValueStore, error) {
			if kvStore, ok := kvStores[namespace]; !ok {
				s := NewTestKeyValueStore(ctrl, slices.Clone[[]byte])
				kvStores[namespace] = s
				return s, nil
			} else {
				return kvStore, nil
			}
		}).
		AnyTimes()
	return mockKvStoreBroker
}

func NewTestKeyValueStore[T any](ctrl *gomock.Controller, clone func(T) T) storage.KeyValueStoreT[T] {
	mockKvStore := NewMockKeyValueStoreT[T](ctrl)
	mu := sync.Mutex{}
	kvs := map[string]T{}
	mockKvStore.EXPECT().
		Put(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string, value T) error {
			mu.Lock()
			defer mu.Unlock()
			kvs[key] = clone(value)
			return nil
		}).
		AnyTimes()
	mockKvStore.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string) (T, error) {
			mu.Lock()
			defer mu.Unlock()
			v, ok := kvs[key]
			if !ok {
				return lo.Empty[T](), storage.ErrNotFound
			}
			return clone(v), nil
		}).
		AnyTimes()
	mockKvStore.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string) error {
			mu.Lock()
			defer mu.Unlock()
			delete(kvs, key)
			return nil
		}).
		AnyTimes()
	mockKvStore.EXPECT().
		ListKeys(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, prefix string) ([]string, error) {
			mu.Lock()
			defer mu.Unlock()
			var keys []string
			for k := range kvs {
				if strings.HasPrefix(k, prefix) {
					keys = append(keys, k)
				}
			}
			return keys, nil
		}).
		AnyTimes()
	return mockKvStore
}

func NewTestRBACStore(ctrl *gomock.Controller) storage.RBACStore {
	mockRBACStore := NewMockRBACStore(ctrl)

	roles := map[string]*v1.Role{}
	rbs := map[string]*v1.RoleBinding{}
	mu := sync.Mutex{}

	mockRBACStore.EXPECT().
		CreateRole(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, role *v1.Role) error {
			mu.Lock()
			defer mu.Unlock()
			roles[role.Id] = role
			return nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		UpdateRole(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *v1.Reference, mutator storage.MutatorFunc[*v1.Role]) (*v1.Role, error) {
			mu.Lock()
			defer mu.Unlock()
			if err, ok := InjectedStorageError(ctx); ok {
				return nil, err
			}
			if _, ok := roles[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			role := roles[ref.Id]
			cloned := proto.Clone(role).(*v1.Role)
			mutator(cloned)
			if _, ok := roles[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			roles[ref.Id] = cloned
			return cloned, nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		DeleteRole(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *v1.Reference) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := roles[ref.Id]; !ok {
				return storage.ErrNotFound
			}
			delete(roles, ref.Id)
			return nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		GetRole(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *v1.Reference) (*v1.Role, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := roles[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			return roles[ref.Id], nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		ListRoles(gomock.Any()).
		DoAndReturn(func(_ context.Context) (*v1.RoleList, error) {
			mu.Lock()
			defer mu.Unlock()
			roleList := &v1.RoleList{}
			for _, role := range roles {
				roleList.Items = append(roleList.Items, role)
			}
			return roleList, nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		CreateRoleBinding(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, rb *v1.RoleBinding) error {
			mu.Lock()
			defer mu.Unlock()
			rbs[rb.Id] = rb
			return nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		UpdateRoleBinding(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *v1.Reference, mutator storage.MutatorFunc[*v1.RoleBinding]) (*v1.RoleBinding, error) {
			mu.Lock()
			defer mu.Unlock()
			if err, ok := InjectedStorageError(ctx); ok {
				return nil, err
			}
			if _, ok := rbs[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			rb := rbs[ref.Id]
			cloned := proto.Clone(rb).(*v1.RoleBinding)
			mutator(cloned)
			if _, ok := rbs[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			rbs[ref.Id] = cloned
			return cloned, nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		DeleteRoleBinding(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *v1.Reference) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := rbs[ref.Id]; !ok {
				return storage.ErrNotFound
			}
			delete(rbs, ref.Id)
			return nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		GetRoleBinding(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *v1.Reference) (*v1.RoleBinding, error) {
			mu.Lock()
			if _, ok := rbs[ref.Id]; !ok {
				mu.Unlock()
				return nil, storage.ErrNotFound
			}
			cloned := proto.Clone(rbs[ref.Id]).(*v1.RoleBinding)
			mu.Unlock()
			storage.ApplyRoleBindingTaints(ctx, mockRBACStore, cloned)
			return cloned, nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		ListRoleBindings(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (*v1.RoleBindingList, error) {
			mu.Lock()
			rbList := &v1.RoleBindingList{}
			for _, rb := range rbs {
				cloned := proto.Clone(rb).(*v1.RoleBinding)
				rbList.Items = append(rbList.Items, cloned)
			}
			mu.Unlock()
			for _, rb := range rbList.Items {
				storage.ApplyRoleBindingTaints(ctx, mockRBACStore, rb)
			}
			return rbList, nil
		}).
		AnyTimes()
	return mockRBACStore
}

func NewTestTokenStore(ctx context.Context, ctrl *gomock.Controller) storage.TokenStore {
	mockTokenStore := NewMockTokenStore(ctrl)

	leaseStore := NewLeaseStore(ctx)
	tks := map[string]*v1.BootstrapToken{}
	mu := sync.Mutex{}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tokenID := <-leaseStore.LeaseExpired():
				mockTokenStore.DeleteToken(ctx, &v1.Reference{
					Id: tokenID,
				})
			}
		}
	}()

	mockTokenStore.EXPECT().
		CreateToken(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ttl time.Duration, opts ...storage.TokenCreateOption) (*v1.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			options := storage.NewTokenCreateOptions()
			options.Apply(opts...)
			t := tokens.NewToken().ToBootstrapToken()
			lease := leaseStore.New(t.TokenID, ttl)
			t.Metadata = &v1.BootstrapTokenMetadata{
				LeaseID:      int64(lease.ID),
				Ttl:          int64(ttl),
				UsageCount:   0,
				Labels:       options.Labels,
				Capabilities: options.Capabilities,
			}
			tks[t.TokenID] = t
			return t, nil
		}).
		AnyTimes()
	mockTokenStore.EXPECT().
		DeleteToken(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *v1.Reference) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := tks[ref.Id]; !ok {
				return storage.ErrNotFound
			}
			delete(tks, ref.Id)
			return nil
		}).
		AnyTimes()
	mockTokenStore.EXPECT().
		GetToken(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *v1.Reference) (*v1.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := tks[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			return tks[ref.Id], nil
		}).
		AnyTimes()
	mockTokenStore.EXPECT().
		ListTokens(gomock.Any()).
		DoAndReturn(func(_ context.Context) ([]*v1.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			tokens := make([]*v1.BootstrapToken, 0, len(tks))
			for _, t := range tks {
				tokens = append(tokens, t)
			}
			return tokens, nil
		}).
		AnyTimes()
	mockTokenStore.EXPECT().
		UpdateToken(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *v1.Reference, mutator storage.MutatorFunc[*v1.BootstrapToken]) (*v1.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := tks[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			token := tks[ref.Id]
			cloned := proto.Clone(token).(*v1.BootstrapToken)
			mutator(cloned)
			if _, ok := tks[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			tks[ref.Id] = cloned
			return cloned, nil
		}).
		AnyTimes()

	return mockTokenStore
}

func NewTestStorageBackend(ctx context.Context, ctrl *gomock.Controller) storage.Backend {
	return &storage.CompositeBackend{
		TokenStore:          NewTestTokenStore(ctx, ctrl),
		ClusterStore:        NewTestClusterStore(ctrl),
		RBACStore:           NewTestRBACStore(ctrl),
		KeyringStoreBroker:  NewTestKeyringStoreBroker(ctrl),
		KeyValueStoreBroker: NewTestKeyValueStoreBroker(ctrl),
	}
}

type Lease struct {
	ID         int64
	Expiration time.Time
	TokenID    string
}

type LeaseStore struct {
	leases       []*Lease
	counter      int64
	mu           sync.Mutex
	refresh      chan struct{}
	leaseExpired chan string
}

func NewLeaseStore(ctx context.Context) *LeaseStore {
	ls := &LeaseStore{
		leases:       []*Lease{},
		counter:      0,
		refresh:      make(chan struct{}),
		leaseExpired: make(chan string, 256),
	}
	go ls.run(ctx)
	return ls
}

func (ls *LeaseStore) New(tokenID string, ttl time.Duration) *Lease {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.counter++
	l := &Lease{
		ID:         ls.counter,
		Expiration: time.Now().Add(ttl),
		TokenID:    tokenID,
	}
	ls.leases = append(ls.leases, l)
	sort.Slice(ls.leases, func(i, j int) bool {
		return ls.leases[i].Expiration.Before(ls.leases[j].Expiration)
	})
	select {
	case ls.refresh <- struct{}{}:
	default:
	}
	return l
}

func (ls *LeaseStore) LeaseExpired() <-chan string {
	return ls.leaseExpired
}

func (ls *LeaseStore) run(ctx context.Context) {
	for {
		ls.mu.Lock()
		count := len(ls.leases)
		ls.mu.Unlock()
		if count == 0 {
			select {
			case <-ctx.Done():
				return
			case <-ls.refresh:
				continue
			}
		}
		ls.mu.Lock()
		firstExpiration := ls.leases[0].Expiration
		ls.mu.Unlock()

		timer := time.NewTimer(time.Until(firstExpiration))
		select {
		case <-ctx.Done():
			return
		case <-ls.refresh:
		case <-time.After(time.Until(firstExpiration)):
			ls.expireFirst()
		}
		if !timer.Stop() {
			<-timer.C
		}
	}
}

func (ls *LeaseStore) expireFirst() {
	ls.mu.Lock()
	l := ls.leases[0]
	ls.leases = ls.leases[1:]
	ls.leaseExpired <- l.TokenID
	ls.mu.Unlock()
}
