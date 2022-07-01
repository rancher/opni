package test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/kralicky/gpkg/sync/atomic"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/storage"
	mock_capability "github.com/rancher/opni/pkg/test/mock/capability"
	mock_ident "github.com/rancher/opni/pkg/test/mock/ident"
	mock_notifier "github.com/rancher/opni/pkg/test/mock/notifier"
	mock_storage "github.com/rancher/opni/pkg/test/mock/storage"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/notifier"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

/******************************************************************************
 * Capabilities                                                               *
 ******************************************************************************/

type CapabilityInfo struct {
	Name              string
	CanInstall        bool
	InstallerTemplate string
}

func (ci *CapabilityInfo) canInstall() error {
	if !ci.CanInstall {
		return errors.New("test error")
	}
	return nil
}

func NewTestCapabilityBackend(
	ctrl *gomock.Controller,
	capBackend *CapabilityInfo,
) capability.Backend {
	backend := mock_capability.NewMockBackend(ctrl)
	backend.EXPECT().
		CanInstall().
		DoAndReturn(capBackend.canInstall).
		AnyTimes()
	backend.EXPECT().
		Install(gomock.Any()).
		Return(nil).
		AnyTimes()
	backend.EXPECT().
		Uninstall(gomock.Any()).
		Return(nil).
		AnyTimes()
	backend.EXPECT().
		InstallerTemplate().
		Return(capBackend.InstallerTemplate).
		AnyTimes()
	return backend
}

func NewTestCapabilityBackendClient(
	ctrl *gomock.Controller,
	capBackend *CapabilityInfo,
) capability.BackendClient {
	client := mock_capability.NewMockBackendClient(ctrl)
	client.EXPECT().
		Info(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&capability.InfoResponse{
			CapabilityName: capBackend.Name,
		}, nil).
		AnyTimes()
	client.EXPECT().
		CanInstall(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*emptypb.Empty, error) {
			return nil, capBackend.canInstall()
		}).
		AnyTimes()
	client.EXPECT().
		Install(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *capability.InstallRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
			return nil, nil
		}).
		AnyTimes()
	client.EXPECT().
		Uninstall(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *capability.UninstallRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
			return nil, nil
		}).
		AnyTimes()
	client.EXPECT().
		InstallerTemplate(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&capability.InstallerTemplateResponse{
			Template: capBackend.InstallerTemplate,
		}, nil).
		AnyTimes()
	return client
}

/******************************************************************************
 * Storage                                                                    *
 ******************************************************************************/

func NewTestClusterStore(ctrl *gomock.Controller) storage.ClusterStore {
	mockClusterStore := mock_storage.NewMockClusterStore(ctrl)

	clusters := map[string]*corev1.Cluster{}
	mu := sync.Mutex{}

	mockClusterStore.EXPECT().
		CreateCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, cluster *corev1.Cluster) error {
			mu.Lock()
			defer mu.Unlock()
			clusters[cluster.Id] = cluster
			return nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		DeleteCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *corev1.Reference) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := clusters[ref.Id]; !ok {
				return storage.ErrNotFound
			}
			delete(clusters, ref.Id)
			return nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		ListClusters(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, matchLabels *corev1.LabelSelector, matchOptions corev1.MatchOptions) (*corev1.ClusterList, error) {
			mu.Lock()
			defer mu.Unlock()
			clusterList := &corev1.ClusterList{}
			selectorPredicate := storage.ClusterSelector{
				LabelSelector: matchLabels,
				MatchOptions:  matchOptions,
			}.Predicate()
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
		DoAndReturn(func(_ context.Context, ref *corev1.Reference) (*corev1.Cluster, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := clusters[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			return clusters[ref.Id], nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		UpdateCluster(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *corev1.Reference, mutator storage.MutatorFunc[*corev1.Cluster]) (*corev1.Cluster, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := clusters[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			cluster := clusters[ref.Id]
			cloned := proto.Clone(cluster).(*corev1.Cluster)
			mutator(cloned)
			if _, ok := clusters[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			clusters[ref.Id] = cloned
			return cloned, nil
		}).
		AnyTimes()
	return mockClusterStore
}

type KeyringStoreHandler = func(prefix string, ref *corev1.Reference) (storage.KeyringStore, error)

func NewTestKeyringStoreBroker(ctrl *gomock.Controller, handler ...KeyringStoreHandler) storage.KeyringStoreBroker {
	mockKeyringStoreBroker := mock_storage.NewMockKeyringStoreBroker(ctrl)
	keyringStores := map[string]storage.KeyringStore{}
	defaultHandler := func(prefix string, ref *corev1.Reference) (storage.KeyringStore, error) {
		if keyringStore, ok := keyringStores[prefix+ref.Id]; !ok {
			s := NewTestKeyringStore(ctrl, prefix, ref)
			keyringStores[prefix+ref.Id] = s
			return s, nil
		} else {
			return keyringStore, nil
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
		DoAndReturn(func(prefix string, ref *corev1.Reference) (storage.KeyringStore, error) {
			if prefix == "gateway-internal" {
				return defaultHandler(prefix, ref)
			}
			return h(prefix, ref)
		}).
		AnyTimes()
	return mockKeyringStoreBroker
}

func NewTestKeyringStore(ctrl *gomock.Controller, prefix string, ref *corev1.Reference) storage.KeyringStore {
	mockKeyringStore := mock_storage.NewMockKeyringStore(ctrl)
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
		DoAndReturn(func(_ context.Context) (keyring.Keyring, error) {
			mu.Lock()
			defer mu.Unlock()
			keyring, ok := keyrings[prefix+ref.Id]
			if !ok {
				return nil, storage.ErrNotFound
			}
			return keyring, nil
		}).
		AnyTimes()
	return mockKeyringStore
}

func NewTestKeyValueStoreBroker(ctrl *gomock.Controller) storage.KeyValueStoreBroker {
	mockKvStoreBroker := mock_storage.NewMockKeyValueStoreBroker(ctrl)
	kvStores := map[string]storage.KeyValueStore{}
	mockKvStoreBroker.EXPECT().
		KeyValueStore(gomock.Any()).
		DoAndReturn(func(namespace string) (storage.KeyValueStore, error) {
			if kvStore, ok := kvStores[namespace]; !ok {
				s := NewTestKeyValueStore(ctrl)
				kvStores[namespace] = s
				return s, nil
			} else {
				return kvStore, nil
			}
		}).
		AnyTimes()
	return mockKvStoreBroker
}

func NewTestKeyValueStore(ctrl *gomock.Controller) storage.KeyValueStore {
	mockKvStore := mock_storage.NewMockKeyValueStore(ctrl)
	mu := sync.Mutex{}
	kvs := map[string][]byte{}
	mockKvStore.EXPECT().
		Put(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string, value []byte) error {
			mu.Lock()
			defer mu.Unlock()
			kvs[key] = value
			return nil
		}).
		AnyTimes()
	mockKvStore.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string) ([]byte, error) {
			mu.Lock()
			defer mu.Unlock()
			v, ok := kvs[key]
			if !ok {
				return nil, storage.ErrNotFound
			}
			return v, nil
		}).
		AnyTimes()
	return mockKvStore
}

func NewTestRBACStore(ctrl *gomock.Controller) storage.RBACStore {
	mockRBACStore := mock_storage.NewMockRBACStore(ctrl)

	roles := map[string]*corev1.Role{}
	rbs := map[string]*corev1.RoleBinding{}
	mu := sync.Mutex{}

	mockRBACStore.EXPECT().
		CreateRole(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, role *corev1.Role) error {
			mu.Lock()
			defer mu.Unlock()
			roles[role.Id] = role
			return nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		DeleteRole(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *corev1.Reference) error {
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
		DoAndReturn(func(_ context.Context, ref *corev1.Reference) (*corev1.Role, error) {
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
		DoAndReturn(func(_ context.Context) (*corev1.RoleList, error) {
			mu.Lock()
			defer mu.Unlock()
			roleList := &corev1.RoleList{}
			for _, role := range roles {
				roleList.Items = append(roleList.Items, role)
			}
			return roleList, nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		CreateRoleBinding(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, rb *corev1.RoleBinding) error {
			mu.Lock()
			defer mu.Unlock()
			rbs[rb.Id] = rb
			return nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		DeleteRoleBinding(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *corev1.Reference) error {
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
		DoAndReturn(func(ctx context.Context, ref *corev1.Reference) (*corev1.RoleBinding, error) {
			mu.Lock()
			if _, ok := rbs[ref.Id]; !ok {
				mu.Unlock()
				return nil, storage.ErrNotFound
			}
			cloned := proto.Clone(rbs[ref.Id]).(*corev1.RoleBinding)
			mu.Unlock()
			storage.ApplyRoleBindingTaints(ctx, mockRBACStore, cloned)
			return cloned, nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		ListRoleBindings(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (*corev1.RoleBindingList, error) {
			mu.Lock()
			rbList := &corev1.RoleBindingList{}
			for _, rb := range rbs {
				cloned := proto.Clone(rb).(*corev1.RoleBinding)
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
	mockTokenStore := mock_storage.NewMockTokenStore(ctrl)

	leaseStore := NewLeaseStore(ctx)
	tks := map[string]*corev1.BootstrapToken{}
	mu := sync.Mutex{}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tokenID := <-leaseStore.LeaseExpired():
				mockTokenStore.DeleteToken(ctx, &corev1.Reference{
					Id: tokenID,
				})
			}
		}
	}()

	mockTokenStore.EXPECT().
		CreateToken(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ttl time.Duration, opts ...storage.TokenCreateOption) (*corev1.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			options := storage.NewTokenCreateOptions()
			options.Apply(opts...)
			t := tokens.NewToken().ToBootstrapToken()
			lease := leaseStore.New(t.TokenID, ttl)
			t.Metadata = &corev1.BootstrapTokenMetadata{
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
		DoAndReturn(func(_ context.Context, ref *corev1.Reference) error {
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
		DoAndReturn(func(_ context.Context, ref *corev1.Reference) (*corev1.BootstrapToken, error) {
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
		DoAndReturn(func(_ context.Context) ([]*corev1.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			tokens := make([]*corev1.BootstrapToken, 0, len(tks))
			for _, t := range tks {
				tokens = append(tokens, t)
			}
			return tokens, nil
		}).
		AnyTimes()
	mockTokenStore.EXPECT().
		UpdateToken(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *corev1.Reference, mutator storage.MutatorFunc[*corev1.BootstrapToken]) (*corev1.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := tks[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			token := tks[ref.Id]
			cloned := proto.Clone(token).(*corev1.BootstrapToken)
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

/******************************************************************************
 * Ident                                                                      *
 ******************************************************************************/

func NewTestIdentProvider(ctrl *gomock.Controller, id string) ident.Provider {
	mockIdent := mock_ident.NewMockProvider(ctrl)
	mockIdent.EXPECT().
		UniqueIdentifier(gomock.Any()).
		Return(id, nil).
		AnyTimes()
	return mockIdent
}

/******************************************************************************
 * Rules                                                                      *
 ******************************************************************************/

func NewTestFinder(ctrl *gomock.Controller, groups func() []rules.RuleGroup) notifier.Finder[rules.RuleGroup] {
	mockRuleFinder := mock_notifier.NewMockFinder[rules.RuleGroup](ctrl)
	mockRuleFinder.EXPECT().
		Find(gomock.Any()).
		DoAndReturn(func(ctx context.Context) ([]rules.RuleGroup, error) {
			return groups(), nil
		}).
		AnyTimes()
	return mockRuleFinder
}

/******************************************************************************
 * Health and Status                                                          *
 ******************************************************************************/

type HealthStore struct {
	health              atomic.Value[*corev1.Health]
	GetHealthShouldFail bool
}

func (hb *HealthStore) SetHealth(health *corev1.Health) {
	hb.health.Store(util.ProtoClone(health))
}

func (hb *HealthStore) GetHealth(context.Context, *emptypb.Empty, ...grpc.CallOption) (*corev1.Health, error) {
	if hb.GetHealthShouldFail {
		return nil, errors.New("error")
	}
	return hb.health.Load(), nil
}

/******************************************************************************
 * Auth                                                                       *
 ******************************************************************************/

func ContextWithAuthorizedID(ctx context.Context, clusterID string) context.Context {
	return context.WithValue(ctx, cluster.ClusterIDKey, clusterID)
}
