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
	DeleteRole(context.Context, *corev1.Reference) error
	GetRole(context.Context, *corev1.Reference) (*corev1.Role, error)
	CreateRoleBinding(context.Context, *corev1.RoleBinding) error
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

type KeyValueStoreT[T any] interface {
	Put(ctx context.Context, key string, value T) error
	Get(ctx context.Context, key string) (T, error)
	Delete(ctx context.Context, key string) error
	ListKeys(ctx context.Context, prefix string) ([]string, error)
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

type HttpTtlCache interface {
	// getter for default cache's configuration
	MaxAge() time.Duration

	Get(key string) (resp []byte, ok bool)
	// If 0 is passed as ttl, the default cache's configuration will be used
	Set(key string, resp []byte)
	Delete(key string)
}
