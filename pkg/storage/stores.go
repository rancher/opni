package storage

import (
	"context"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/keyring"
)

type Backend interface {
	TokenStore
	ClusterStore
	RBACStore
	KeyringStoreBroker
	KeyValueStoreBroker
}

type MutatorFunc[T any] func(T)

type TokenMutator = MutatorFunc[*corev1.BootstrapToken]
type ClusterMutator = MutatorFunc[*corev1.Cluster]
type RoleMutator = MutatorFunc[*corev1.Role]
type RoleBindingMutator = MutatorFunc[*corev1.RoleBinding]

type TokenStore interface {
	CreateToken(ctx context.Context, ttl time.Duration, opts ...TokenCreateOption) (*corev1.BootstrapToken, error)
	DeleteToken(ctx context.Context, ref *corev1.Reference) error
	GetToken(ctx context.Context, ref *corev1.Reference) (*corev1.BootstrapToken, error)
	UpdateToken(ctx context.Context, ref *corev1.Reference, mutator TokenMutator) (*corev1.BootstrapToken, error)
	ListTokens(ctx context.Context) ([]*corev1.BootstrapToken, error)
}

type ClusterStore interface {
	CreateCluster(ctx context.Context, cluster *corev1.Cluster) error
	DeleteCluster(ctx context.Context, ref *corev1.Reference) error
	GetCluster(ctx context.Context, ref *corev1.Reference) (*corev1.Cluster, error)
	UpdateCluster(ctx context.Context, ref *corev1.Reference, mutator ClusterMutator) (*corev1.Cluster, error)
	WatchCluster(ctx context.Context, cluster *corev1.Cluster) (<-chan WatchEvent[*corev1.Cluster], error)
	WatchClusters(ctx context.Context, known []*corev1.Cluster) (<-chan WatchEvent[*corev1.Cluster], error)
	ListClusters(ctx context.Context, matchLabels *corev1.LabelSelector, matchOptions corev1.MatchOptions) (*corev1.ClusterList, error)
}

type RBACStore interface {
	CreateRole(context.Context, *corev1.Role) error
	UpdateRole(ctx context.Context, ref *corev1.Reference, mutator RoleMutator) (*corev1.Role, error)
	DeleteRole(context.Context, *corev1.Reference) error
	GetRole(context.Context, *corev1.Reference) (*corev1.Role, error)
	CreateRoleBinding(context.Context, *corev1.RoleBinding) error
	UpdateRoleBinding(ctx context.Context, ref *corev1.Reference, mutator RoleBindingMutator) (*corev1.RoleBinding, error)
	DeleteRoleBinding(context.Context, *corev1.Reference) error
	GetRoleBinding(context.Context, *corev1.Reference) (*corev1.RoleBinding, error)
	ListRoles(context.Context) (*corev1.RoleList, error)
	ListRoleBindings(context.Context) (*corev1.RoleBindingList, error)
}

type KeyringStore interface {
	Put(ctx context.Context, keyring keyring.Keyring) error
	Get(ctx context.Context) (keyring.Keyring, error)
	Delete(ctx context.Context) error
}

type KeyRevision[T any] interface {
	Key() string
	// If values were requested, returns the value at this revision. Otherwise,
	// returns nil.
	Value() T
	// Returns the revision of this key. Larger values are newer, but the
	// revision number should otherwise be treated as an opaque value.
	Revision() int64
	// Returns the timestamp of this revision. This may or may not always be
	// available, depending on if the underlying store supports it.
	Timestamp() time.Time
}

type KeyRevisionImpl[T any] struct {
	K    string
	V    T
	Rev  int64
	Time time.Time
}

func (k *KeyRevisionImpl[T]) Key() string {
	return k.K
}

func (k *KeyRevisionImpl[T]) Value() T {
	return k.V
}

func (k *KeyRevisionImpl[T]) Revision() int64 {
	return k.Rev
}

func (k *KeyRevisionImpl[T]) Timestamp() time.Time {
	return k.Time
}

type KeyValueStoreT[T any] interface {
	Put(ctx context.Context, key string, value T, opts ...PutOpt) error
	Get(ctx context.Context, key string, opts ...GetOpt) (T, error)
	Delete(ctx context.Context, key string, opts ...DeleteOpt) error
	ListKeys(ctx context.Context, prefix string, opts ...ListOpt) ([]string, error)
	History(ctx context.Context, key string, opts ...HistoryOpt) ([]KeyRevision[T], error)
}

type ValueStoreT[T any] interface {
	Put(ctx context.Context, value T, opts ...PutOpt) error
	Get(ctx context.Context, opts ...GetOpt) (T, error)
	Delete(ctx context.Context, opts ...DeleteOpt) error
	History(ctx context.Context, opts ...HistoryOpt) ([]KeyRevision[T], error)
}

type KeyValueStore KeyValueStoreT[[]byte]

type KeyringStoreBroker interface {
	KeyringStore(namespace string, ref *corev1.Reference) KeyringStore
}

type KeyValueStoreBroker interface {
	KeyValueStore(namespace string) KeyValueStore
}

// A store that can be used to compute subject access rules
type SubjectAccessCapableStore interface {
	ListClusters(ctx context.Context, matchLabels *corev1.LabelSelector, matchOptions corev1.MatchOptions) (*corev1.ClusterList, error)
	GetRole(ctx context.Context, ref *corev1.Reference) (*corev1.Role, error)
	ListRoleBindings(ctx context.Context) (*corev1.RoleBindingList, error)
}

type WatchEventType string

const (
	WatchEventCreate WatchEventType = "PUT"
	WatchEventUpdate WatchEventType = "UPDATE"
	WatchEventDelete WatchEventType = "DELETE"
)

type WatchEvent[T any] struct {
	EventType WatchEventType
	Current   T
	Previous  T
}

type HttpTtlCache[T any] interface {
	// getter for default cache's configuration
	MaxAge() time.Duration

	Get(key string) (resp T, ok bool)
	// If 0 is passed as ttl, the default cache's configuration will be used
	Set(key string, resp T)
	Delete(key string)
}

type GrpcTtlCache[T any] interface {
	// getter for default cache's configuration
	MaxAge() time.Duration

	Get(key string) (resp T, ok bool)
	// If 0 is passed as ttl, the default cache's configuration will be used
	Set(key string, resp T, ttl time.Duration)
	Delete(key string)
}

var (
	storeBuilderCache = map[string]func(...any) (any, error){}
)

func RegisterStoreBuilder[T ~string](name T, builder func(...any) (any, error)) {
	storeBuilderCache[string(name)] = builder
}

func GetStoreBuilder[T ~string](name T) func(...any) (any, error) {
	return storeBuilderCache[string(name)]
}
