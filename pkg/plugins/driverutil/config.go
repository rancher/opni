package driverutil

import (
	"context"
	"fmt"
	"sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/fieldmask"
	"github.com/rancher/opni/pkg/util/merge"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type DefaultLoaderFunc[T any] func(T)

type DryRunResults[T any] struct {
	Current  T
	Modified T
}

type DefaultingConfigTracker[T ConfigType[T]] struct {
	lock               *sync.Mutex
	defaultStore       storage.ValueStoreT[T]
	activeStore        storage.ValueStoreT[T]
	defaultLoader      DefaultLoaderFunc[T]
	revisionFieldIndex int

	redact   func(SecretsRedactor[T])
	unredact func(SecretsRedactor[T], T) error
}

func NewDefaultingConfigTracker[T ConfigType[T]](
	defaultStore, activeStore storage.ValueStoreT[T],
	loadDefaultsFunc DefaultLoaderFunc[T],
) *DefaultingConfigTracker[T] {
	return &DefaultingConfigTracker[T]{
		lock:               &sync.Mutex{},
		defaultStore:       defaultStore,
		activeStore:        activeStore,
		defaultLoader:      loadDefaultsFunc,
		revisionFieldIndex: GetRevisionFieldIndex[T](),
		redact:             (SecretsRedactor[T]).RedactSecrets,
		unredact:           (SecretsRedactor[T]).UnredactSecrets,
	}
}

func (ct *DefaultingConfigTracker[T]) newDefaultSpec() (t T) {
	t = t.ProtoReflect().New().Interface().(T)
	ct.defaultLoader(t)
	return t
}

// Gets the default config if one has been set, otherwise returns a new default
// config as defined by the type.
func (ct *DefaultingConfigTracker[T]) GetDefaultConfig(ctx context.Context, atRevision ...*corev1.Revision) (T, error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	def, rev, err := ct.getDefaultConfigLocked(ctx, atRevision...)
	if err != nil {
		return def, err
	}

	ct.redact(def)
	SetRevision(def, rev)
	return def, nil
}

// Sets the default config directly. No merging is performed.
func (ct *DefaultingConfigTracker[T]) SetDefaultConfig(ctx context.Context, newDefault T) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	newDefault = util.ProtoClone(newDefault)
	newDefaultRevision := newDefault.GetRevision().GetRevision()

	// don't care about the revision here, it should match the revision of the
	// input if the operation is valid
	existing, _, err := ct.getDefaultConfigLocked(ctx)
	if err != nil {
		return err
	}
	if err := ct.unredact(newDefault, existing); err != nil {
		return err
	}

	UnsetRevision(newDefault)
	return ct.defaultStore.Put(ctx, newDefault, storage.WithRevision(newDefaultRevision))
}

// Deletes the default config, leaving it unset. Subsequent calls to GetDefaultConfig
// will return a new default config as defined by the type.
func (ct *DefaultingConfigTracker[T]) ResetDefaultConfig(ctx context.Context, atRevision ...*corev1.Revision) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	opts := []storage.DeleteOpt{}
	if len(atRevision) > 0 {
		opts = append(opts, storage.WithRevision(*atRevision[0].Revision))
	}
	if err := ct.defaultStore.Delete(ctx, opts...); err != nil {
		return fmt.Errorf("error resetting config: %w", err)
	}
	return nil
}

func (ct *DefaultingConfigTracker[T]) getDefaultConfigLocked(ctx context.Context, atRevision ...*corev1.Revision) (T, int64, error) {
	var revision int64
	opts := []storage.GetOpt{
		storage.WithRevisionOut(&revision),
	}
	opts = maybeWithRevision(atRevision, opts)
	def, err := ct.defaultStore.Get(ctx, opts...)
	if err != nil {
		if !storage.IsNotFound(err) {
			return def, 0, fmt.Errorf("error looking up default config: %w", err)
		}
		def = ct.newDefaultSpec()
	}
	return def, revision, nil
}

// Returns the active config if it has been set, otherwise returns a "not found" error.
// An optional revision can be provided to get the config at a specific revision.
func (ct *DefaultingConfigTracker[T]) GetConfig(ctx context.Context, atRevision ...*corev1.Revision) (T, error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	var revision int64
	opts := []storage.GetOpt{
		storage.WithRevisionOut(&revision),
	}
	opts = maybeWithRevision(atRevision, opts)
	existing, err := ct.activeStore.Get(ctx, opts...)
	if err != nil {
		return existing, fmt.Errorf("error looking up config: %w", err)
	}
	ct.redact(existing)
	SetRevision(existing, revision)
	return existing, nil
}

// Restores the active config to match the default config. An optional field mask
// can be provided to specify which fields to keep from the current config.
// If a nil mask is given, the active config (potentially including history)
// is deleted from the underlying store. If a non-nil mask is given, the active
// config is only modified (preserving history), not deleted.
func (ct *DefaultingConfigTracker[T]) ResetConfig(ctx context.Context, mask *fieldmaskpb.FieldMask, patch T, atRevision ...*corev1.Revision) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	var revision int64
	opts := []storage.GetOpt{
		storage.WithRevisionOut(&revision),
	}
	opts = maybeWithRevision(atRevision, opts)
	activeConfig, err := ct.activeStore.Get(ctx, opts...)
	if err != nil {
		return fmt.Errorf("error looking up config: %w", err)
	}
	if mask == nil {
		err := ct.activeStore.Delete(ctx, storage.WithRevision(revision))
		if err != nil {
			return fmt.Errorf("error deleting config: %w", err)
		}
		return nil
	}
	defaultConfig, _, err := ct.getDefaultConfigLocked(ctx)
	if err != nil {
		return err
	}
	mask.Normalize()
	if !mask.IsValid(activeConfig) {
		return status.Errorf(codes.InvalidArgument, "invalid field mask: %v", mask.GetPaths())
	}
	for i, path := range mask.GetPaths() {
		if path == "" {
			// empty paths in field masks can be destructive and are never intentional
			return status.Errorf(codes.InvalidArgument, "field mask contains an empty path at index %d", i)
		}
	}

	fieldmask.ExclusiveKeep(activeConfig, mask)
	if err := ct.unredact(patch, activeConfig); err != nil {
		return err
	}
	fieldmask.ExclusiveKeep(patch, mask)

	merge.MergeWithReplace(activeConfig, patch)
	merge.MergeWithReplace(defaultConfig, activeConfig)

	return ct.activeStore.Put(ctx, defaultConfig, storage.WithRevision(revision))
}

// Returns the active config if it has been set, otherwise returns the default config.
// The optional revision only applies to the active config; if it does not exist,
// the default config will always be at the latest revision.
func (ct *DefaultingConfigTracker[T]) GetConfigOrDefault(ctx context.Context, atRevision ...*corev1.Revision) (T, error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	value, rev, err := ct.getConfigOrDefaultLocked(ctx, atRevision...)
	if err != nil {
		return value, err
	}
	ct.redact(value)

	SetRevision(value, rev)
	return value, nil
}

func (ct *DefaultingConfigTracker[T]) getConfigOrDefaultLocked(ctx context.Context, atRevision ...*corev1.Revision) (T, int64, error) {
	defaultValue, defaultErr := ct.defaultStore.Get(ctx)
	var activeRevision int64
	opts := []storage.GetOpt{
		storage.WithRevisionOut(&activeRevision),
	}
	opts = maybeWithRevision(atRevision, opts)
	activeValue, activeErr := ct.activeStore.Get(ctx, opts...)
	if activeErr != nil {
		if !storage.IsNotFound(activeErr) {
			return activeValue, 0, fmt.Errorf("error looking up config: %w", activeErr)
		}
		// NB: we only save the revision from the active store, because the
		// return value of this function is intended to be used as the
		// active config. if it's unset, the revision should be set to 0,
		// so that it will only be usable as an active config, but would be
		// rejected as a default config.
		if defaultErr != nil {
			if !storage.IsNotFound(defaultErr) {
				return defaultValue, 0, fmt.Errorf("error looking up default config: %w", activeErr)
			}
			return ct.newDefaultSpec(), 0, nil
		}
		return defaultValue, 0, nil
	}
	return activeValue, activeRevision, nil
}

func maybeWithRevision(atRevision []*corev1.Revision, opts []storage.GetOpt) []storage.GetOpt {
	if len(atRevision) > 0 && atRevision[0] != nil && atRevision[0].Revision != nil {
		opts = append(opts, storage.WithRevision(*atRevision[0].Revision))
	}
	return opts
}

// ApplyConfig sets the active config by merging the given config onto the existing
// active config, or onto the default config if no active config has been set.
func (ct *DefaultingConfigTracker[T]) ApplyConfig(ctx context.Context, newConfig T) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	existing, rev, err := ct.getConfigOrDefaultLocked(ctx, newConfig.GetRevision())
	if err != nil {
		return err
	}

	if err := ct.unredact(newConfig, existing); err != nil {
		return err
	}

	merge.MergeWithReplace(existing, newConfig)

	UnsetRevision(existing)
	return ct.activeStore.Put(ctx, existing, storage.WithRevision(rev))
}

func (ct *DefaultingConfigTracker[T]) DryRun(ctx context.Context, req DryRunRequestType[T]) (*DryRunResults[T], error) {
	switch req.GetTarget() {
	case Target_ActiveConfiguration:
		switch req.GetAction() {
		case Action_Set:
			res, err := ct.DryRunApplyConfig(ctx, req.GetSpec())
			if err != nil {
				return nil, err
			}
			return &DryRunResults[T]{
				Current:  res.Current,
				Modified: res.Modified,
			}, nil
		case Action_Reset:
			res, err := ct.DryRunResetConfig(ctx, req.GetMask(), req.GetPatch(), req.GetRevision())
			if err != nil {
				return nil, err
			}
			return &DryRunResults[T]{
				Current:  res.Current,
				Modified: res.Modified,
			}, nil
		default:
			return nil, fmt.Errorf("invalid action: %s", req.GetAction())
		}
	case Target_DefaultConfiguration:
		switch req.GetAction() {
		case Action_Set:
			res, err := ct.DryRunSetDefaultConfig(ctx, req.GetSpec())
			if err != nil {
				return nil, err
			}
			return &DryRunResults[T]{
				Current:  res.Current,
				Modified: res.Modified,
			}, nil
		case Action_Reset:
			res, err := ct.DryRunResetDefaultConfig(ctx, req.GetRevision())
			if err != nil {
				return nil, err
			}
			return &DryRunResults[T]{
				Current:  res.Current,
				Modified: res.Modified,
			}, nil
		default:
			return nil, fmt.Errorf("invalid action: %s", req.GetAction())
		}
	default:
		return nil, fmt.Errorf("invalid target: %s", req.GetTarget())
	}
}

func (ct *DefaultingConfigTracker[T]) History(ctx context.Context, target Target, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
	var targetStore storage.ValueStoreT[T]
	switch target {
	case Target_ActiveConfiguration:
		targetStore = ct.activeStore
	case Target_DefaultConfiguration:
		targetStore = ct.defaultStore
	default:
		return nil, fmt.Errorf("invalid target: %s", target)
	}
	revisions, err := targetStore.History(ctx, opts...)
	if err != nil {
		return nil, err
	}
	for _, rev := range revisions {
		rev.Value().RedactSecrets()
	}
	return revisions, nil
}

func (ct *DefaultingConfigTracker[T]) DryRunApplyConfig(ctx context.Context, newConfig T) (DryRunResults[T], error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	current, rev, err := ct.getConfigOrDefaultLocked(ctx)
	if err != nil {
		return DryRunResults[T]{}, err
	}

	if newConfig.GetRevision() != nil && newConfig.GetRevision().GetRevision() != rev {
		return DryRunResults[T]{}, storage.ErrConflict
	}

	if err := ct.unredact(newConfig, current); err != nil {
		return DryRunResults[T]{}, err
	}

	modified := util.ProtoClone(current)

	merge.MergeWithReplace(modified, newConfig)

	ct.redact(current)
	ct.redact(modified)

	// Unset the revision for the modified config. Revisions are not stored
	// in the actual object inside the kv store, so they should not be included
	// in the diff.
	UnsetRevision(modified)
	return DryRunResults[T]{
		Current:  current,
		Modified: modified,
	}, nil
}

func (ct *DefaultingConfigTracker[T]) DryRunSetDefaultConfig(ctx context.Context, newDefault T) (DryRunResults[T], error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	current, rev, err := ct.getDefaultConfigLocked(ctx)
	if err != nil {
		return DryRunResults[T]{}, err
	}
	if newDefault.GetRevision() != nil && newDefault.GetRevision().GetRevision() != rev {
		return DryRunResults[T]{}, storage.ErrConflict
	}

	if err := ct.unredact(newDefault, current); err != nil {
		return DryRunResults[T]{}, err
	}

	ct.redact(current)
	ct.redact(newDefault)

	return DryRunResults[T]{
		Current:  current,
		Modified: newDefault,
	}, nil
}

func (ct *DefaultingConfigTracker[T]) DryRunResetDefaultConfig(ctx context.Context, atRevision ...*corev1.Revision) (DryRunResults[T], error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	current, err := ct.defaultStore.Get(ctx, maybeWithRevision(atRevision, nil)...)
	if err != nil {
		// always return an error here, including not found errors, since
		// dry run reset only makes sense if there is an overridden default
		// config to reset
		return DryRunResults[T]{}, err
	}

	newDefault := ct.newDefaultSpec()

	ct.redact(current)
	ct.redact(newDefault)
	return DryRunResults[T]{
		Current:  current,
		Modified: newDefault,
	}, nil
}

func (ct *DefaultingConfigTracker[T]) DryRunResetConfig(ctx context.Context, mask *fieldmaskpb.FieldMask, patch T, atRevision ...*corev1.Revision) (DryRunResults[T], error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	activeConfig, err := ct.activeStore.Get(ctx, maybeWithRevision(atRevision, nil)...)
	if err != nil {
		return DryRunResults[T]{}, err
	}
	defaultConfig, _, err := ct.getDefaultConfigLocked(ctx)
	if err != nil {
		return DryRunResults[T]{}, err
	}
	if mask == nil {
		ct.redact(activeConfig)
		ct.redact(defaultConfig)
		return DryRunResults[T]{
			Current:  activeConfig,
			Modified: defaultConfig,
		}, nil
	}

	mask.Normalize()
	if !mask.IsValid(activeConfig) {
		return DryRunResults[T]{}, fmt.Errorf("invalid field mask: %v", mask.GetPaths())
	}
	for i, path := range mask.GetPaths() {
		if path == "" {
			// empty paths in field masks can be destructive and are never intentional
			return DryRunResults[T]{}, status.Errorf(codes.InvalidArgument, "field mask contains an empty path at index %d", i)
		}
	}

	originalCurrent := util.ProtoClone(activeConfig)
	fieldmask.ExclusiveKeep(activeConfig, mask)
	if err := ct.unredact(patch, activeConfig); err != nil {
		return DryRunResults[T]{}, err
	}
	fieldmask.ExclusiveKeep(patch, mask)

	merge.MergeWithReplace(activeConfig, patch)
	merge.MergeWithReplace(defaultConfig, activeConfig)

	ct.redact(originalCurrent)
	ct.redact(defaultConfig)
	return DryRunResults[T]{
		Current:  originalCurrent,
		Modified: defaultConfig,
	}, nil
}

type contextKeyedValueStore[T ConfigType[T]] struct {
	base storage.KeyValueStoreT[T]
}

type contextKeyedValueStore_keyType struct{}

var contextKeyedValueStore_key contextKeyedValueStore_keyType

func contextWithKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, contextKeyedValueStore_key, key)
}

func keyFromContext(ctx context.Context) string {
	return ctx.Value(contextKeyedValueStore_key).(string)
}

func (s *contextKeyedValueStore[T]) Put(ctx context.Context, value T, opts ...storage.PutOpt) error {
	return s.base.Put(ctx, keyFromContext(ctx), value, opts...)
}

func (s *contextKeyedValueStore[T]) Get(ctx context.Context, opts ...storage.GetOpt) (T, error) {
	return s.base.Get(ctx, keyFromContext(ctx), opts...)
}

func (s *contextKeyedValueStore[T]) Watch(ctx context.Context, opts ...storage.WatchOpt) (<-chan storage.WatchEvent[storage.KeyRevision[T]], error) {
	return s.base.Watch(ctx, keyFromContext(ctx), opts...)
}

func (s *contextKeyedValueStore[T]) Delete(ctx context.Context, opts ...storage.DeleteOpt) error {
	return s.base.Delete(ctx, keyFromContext(ctx), opts...)
}

func (s *contextKeyedValueStore[T]) History(ctx context.Context, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
	return s.base.History(ctx, keyFromContext(ctx), opts...)
}

// A config tracker that uses a single default store and many active stores,
// each with a unique key.
func NewDefaultingActiveKeyedConfigTracker[T ConfigType[T]](
	defaultStore storage.ValueStoreT[T],
	activeStore storage.KeyValueStoreT[T],
	loadDefaultsFunc DefaultLoaderFunc[T],
) *DefaultingConfigTracker[T] {
	return NewDefaultingConfigTracker[T](defaultStore, &contextKeyedValueStore[T]{
		base: activeStore,
	}, loadDefaultsFunc)
}
