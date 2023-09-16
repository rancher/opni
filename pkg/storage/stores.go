package storage

import (
	"context"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage/lock"
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
	SetKey(string)

	// If values were requested, returns the value at this revision. Otherwise,
	// returns the zero value for T.
	// Note that if the value has a revision field, it will *not*
	// be populated, and should be set manually if needed using the Revision()
	// method.
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

func (k *KeyRevisionImpl[T]) SetKey(key string) {
	k.K = key
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

	// Starts a watch on the specified key. The returned channel will receive
	// events for the key until the context is canceled, after which the
	// channel will be closed. This function does not block. An error will only
	// be returned if the key is invalid or the watch fails to start.
	//
	// When the watch is started, the current value of the key will be sent
	// if and only if both of the following conditions are met:
	// 1. A revision is explicitly set in the watch options. If no revision is
	//    specified, only future events will be sent. Revision 0 is equivalent
	//    to the oldest revision among all keys matching the prefix, not
	//    including deleted keys.
	// 2. The key exists; or in prefix mode, there is at least one key matching
	//    the prefix.
	//
	// In most cases a starting revision should be specified, as this will
	// ensure no events are missed.
	//
	// This function can be called multiple times for the same key, prefix, or
	// overlapping prefixes. Each call will initiate a separate watch, and events
	// are always replicated to all active watches.
	//
	// The channels are buffered to hold at least 64 events. Ensure that events
	// are read from the channel in a timely manner if a large volume of events
	// are expected; otherwise it will block and events may be delayed, or be
	// dropped by the backend.
	Watch(ctx context.Context, key string, opts ...WatchOpt) (<-chan WatchEvent[KeyRevision[T]], error)
	Delete(ctx context.Context, key string, opts ...DeleteOpt) error
	ListKeys(ctx context.Context, prefix string, opts ...ListOpt) ([]string, error)
	History(ctx context.Context, key string, opts ...HistoryOpt) ([]KeyRevision[T], error)
}

type ValueStoreT[T any] interface {
	Put(ctx context.Context, value T, opts ...PutOpt) error
	Get(ctx context.Context, opts ...GetOpt) (T, error)
	Watch(ctx context.Context, opts ...WatchOpt) (<-chan WatchEvent[KeyRevision[T]], error)
	Delete(ctx context.Context, opts ...DeleteOpt) error
	History(ctx context.Context, opts ...HistoryOpt) ([]KeyRevision[T], error)
}

type KeyValueStore = KeyValueStoreT[[]byte]

type KeyringStoreBroker interface {
	KeyringStore(namespace string, ref *corev1.Reference) KeyringStore
}

type KeyValueStoreBroker interface {
	KeyValueStore(namespace string) KeyValueStore
}

type KeyValueStoreTBroker[T any] interface {
	KeyValueStore(namespace string) KeyValueStoreT[T]
}

type LockManagerBroker interface {
	LockManager(namespace string) LockManager
}

// A store that can be used to compute subject access rules
type SubjectAccessCapableStore interface {
	ListClusters(ctx context.Context, matchLabels *corev1.LabelSelector, matchOptions corev1.MatchOptions) (*corev1.ClusterList, error)
	GetRole(ctx context.Context, ref *corev1.Reference) (*corev1.Role, error)
	ListRoleBindings(ctx context.Context) (*corev1.RoleBindingList, error)
}

type WatchEventType string

// Lock is a distributed lock that can be used to coordinate access to a resource.
//
// Locks are single use, and return errors when used more than once. Retry mechanisms are built into\
// the Lock and can be configured with LockOptions.
type Lock interface {
	// Lock acquires a lock on the key. If the lock is already held, it will block until the lock is released.\
	//
	// Lock returns an error when acquiring the lock fails.
	Lock() error
	// Unlock releases the lock on the key. If the lock was never held, it will return an error.
	Unlock() error
	// Key returns a unique temporary prefix key that exists while the lock is held.
	// If the lock is not currently held, the return value is undefined.
	Key() string
}

// LockManager replaces sync.Mutex when a distributed locking mechanism is required.
//
// # Usage
//
// ## Transient lock (transactions, tasks, ...)
// ```
//
//		func distributedTransation(lm stores.LockManager, key string) error {
//			keyMu := lm.Locker(key, lock.WithKeepalive(false), lock.WithExpireDuration(1 * time.Second))
//			if err := keyMu.Lock(); err != nil {
//			return err
//			}
//			defer keyMu.Unlock()
//	     // do some work
//		}
//
// ```
//
// ## Persistent lock (leader election)
//
// ```
//
//	func serve(lm stores.LockManager, key string, done chan struct{}) error {
//		keyMu := lm.Locker(key, lock.WithKeepalive(true))
//		if err := keyMu.Lock(); err != nil {
//			return err
//		}
//		go func() {
//			<-done
//			keyMu.Unlock()
//		}()
//		// serve a service...
//	}
//
// ```
type LockManager interface {
	// Instantiates a new Lock instance for the given key, with the given options.
	//
	// Defaults to lock.DefaultOptions if no options are provided.
	Locker(key string, opts ...lock.LockOption) Lock
}

const (
	// An operation that creates a new key OR modifies an existing key.
	//
	// NB: The Watch API does not distinguish between create and modify events.
	// It is not practical (nor desired, in most cases) to provide this info
	// to the caller, because it cannot be guaranteed to be accurate in all cases.
	// Because of the inability to make this guarantee, any client code that
	// relies on this distinction would be highly likely to end up in an invalid
	// state after a sufficient amount of time, or after issuing a watch request
	// on a key that has a complex and/or truncated history. However, in certain
	// cases, clients may be able to correlate events with out-of-band information
	// to reliably disambiguate Put events. This is necessarily an implementation
	// detail and may not always be possible.
	WatchEventPut WatchEventType = "Put"

	// An operation that removes an existing key.
	//
	// Delete events make few guarantees, as different backends handle deletes
	// differently. Backends are not required to discard revision history, or
	// to stop sending events for a key after it has been deleted. Keys may
	// be recreated after a delete event, in which case a Put event will follow.
	// Such events may or may not contain a previous revision value, depending
	// on implementation details of the backend (they will always contain a
	// current revision value, though).
	WatchEventDelete WatchEventType = "Delete"
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
