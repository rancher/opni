package driverutil

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/mennanov/fmutils"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/merge"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type DefaultLoaderFunc[T any] func(T)

type secretsRedactor[T any] interface {
	RedactSecrets()
	UnredactSecrets(T) error
}

type config_type[T any] interface {
	proto.Message
	GetRevision() *corev1.Revision
	secretsRedactor[T]
}

type DryRunResults[T any] struct {
	Current  T
	Modified T
}

type DefaultingConfigTracker[T config_type[T]] struct {
	lock               sync.Mutex
	defaultStore       storage.ValueStoreT[T]
	activeStore        storage.ValueStoreT[T]
	defaultLoader      DefaultLoaderFunc[T]
	revisionFieldIndex int
}

func NewDefaultingConfigTracker[T config_type[T]](
	defaultStore, activeStore storage.ValueStoreT[T],
	loadDefaultsFunc DefaultLoaderFunc[T],
) *DefaultingConfigTracker[T] {
	return &DefaultingConfigTracker[T]{
		defaultStore:       defaultStore,
		activeStore:        activeStore,
		defaultLoader:      loadDefaultsFunc,
		revisionFieldIndex: getRevisionFieldIndex[T](),
	}
}

func (ct *DefaultingConfigTracker[T]) newDefaultSpec() (t T) {
	t = t.ProtoReflect().New().Interface().(T)
	ct.defaultLoader(t)
	return t
}

// Gets the default config if one has been set, otherwise returns a new default
// config as defined by the type.
func (ct *DefaultingConfigTracker[T]) GetDefaultConfig(ctx context.Context) (T, error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	def, rev, err := ct.getDefaultConfigLocked(ctx)
	if err != nil {
		return def, err
	}

	def.RedactSecrets()
	ct.setRevision(def, rev)
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
	if err := newDefault.UnredactSecrets(existing); err != nil {
		return err
	}

	ct.unsetRevision(newDefault)
	return ct.defaultStore.Put(ctx, newDefault, storage.WithRevision(newDefaultRevision))
}

// Deletes the default config, leaving it unset. Subsequent calls to GetDefaultConfig
// will return a new default config as defined by the type.
func (ct *DefaultingConfigTracker[T]) ResetDefaultConfig(ctx context.Context) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	if err := ct.defaultStore.Delete(ctx); err != nil {
		return fmt.Errorf("error resetting config: %w", err)
	}
	return nil
}

func (ct *DefaultingConfigTracker[T]) getDefaultConfigLocked(ctx context.Context) (T, int64, error) {
	var revision int64
	def, err := ct.defaultStore.Get(ctx, storage.WithRevisionOut(&revision))
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return def, 0, fmt.Errorf("error looking up default config: %w", err)
		}
		def = ct.newDefaultSpec()
	}
	return def, revision, nil
}

// Returns the active config if it has been set, otherwise returns a "not found" error.
func (ct *DefaultingConfigTracker[T]) GetConfig(ctx context.Context) (T, error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	var revision int64
	existing, err := ct.activeStore.Get(ctx, storage.WithRevisionOut(&revision))
	if err != nil {
		return existing, fmt.Errorf("error looking up config: %w", err)
	}
	existing.RedactSecrets()
	ct.setRevision(existing, revision)
	return existing, nil
}

// Restores the active config to match the default config. An optional field mask
// can be provided to specify which fields to keep from the current config.
// If no field mask is provided, the active config may be deleted from the store.
func (ct *DefaultingConfigTracker[T]) ResetConfig(ctx context.Context, keep *fieldmaskpb.FieldMask) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	var revision int64
	current, err := ct.activeStore.Get(ctx, storage.WithRevisionOut(&revision))
	if err != nil {
		return fmt.Errorf("error looking up config: %w", err)
	}
	if len(keep.GetPaths()) == 0 {
		err := ct.activeStore.Delete(ctx, storage.WithRevision(revision))
		if err != nil {
			return fmt.Errorf("error deleting config: %w", err)
		}
		return nil
	}

	defaultConfig, _, err := ct.getDefaultConfigLocked(ctx)
	keep.Normalize()
	if !keep.IsValid(current) {
		return fmt.Errorf("invalid field mask: %v", keep.GetPaths())
	}

	fmutils.Filter(current, keep.GetPaths())

	merge.MergeWithReplace(defaultConfig, current)

	return ct.activeStore.Put(ctx, defaultConfig, storage.WithRevision(revision))
}

// Returns the active config if it has been set, otherwise returns the default config.
func (ct *DefaultingConfigTracker[T]) GetConfigOrDefault(ctx context.Context) (T, error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	value, rev, err := ct.getConfigOrDefaultLocked(ctx)
	if err != nil {
		return value, err
	}
	value.RedactSecrets()

	ct.setRevision(value, rev)
	return value, nil
}

func (ct *DefaultingConfigTracker[T]) getConfigOrDefaultLocked(ctx context.Context) (T, int64, error) {
	var revision int64
	value, err := ct.activeStore.Get(ctx, storage.WithRevisionOut(&revision))
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return value, 0, fmt.Errorf("error looking up config: %w", err)
		}
		// NB: we only save the revision from the active store, because the
		// return value of this function is intended to be used as the
		// active config. if it's unset, the revision should be set to 0,
		// so that it will only be usable as an active config, but would be
		// rejected as a default config.
		value, err = ct.defaultStore.Get(ctx)
		if err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				return value, 0, fmt.Errorf("error looking up default config: %w", err)
			}
			value = ct.newDefaultSpec() // no modifications are made to revision
		}
	}
	return value, revision, nil
}

// ApplyConfig sets the active config by merging the given config onto the existing
// active config, or onto the default config if no active config has been set.
func (ct *DefaultingConfigTracker[T]) ApplyConfig(ctx context.Context, newConfig T) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	var newConfigRevision *int64
	if newConfig.GetRevision() != nil {
		newConfigRevision = newConfig.GetRevision().Revision
	}

	existing, rev, err := ct.getConfigOrDefaultLocked(ctx)
	if err != nil {
		return err
	}
	if newConfigRevision == nil {
		newConfigRevision = &rev
	}
	if err := newConfig.UnredactSecrets(existing); err != nil {
		return err
	}

	merge.MergeWithReplace(existing, newConfig)

	ct.unsetRevision(existing)
	return ct.activeStore.Put(ctx, existing, storage.WithRevision(*newConfigRevision))
}

func (ct *DefaultingConfigTracker[T]) DryRun(ctx context.Context, target Target, action Action, spec T) (*DryRunResults[T], error) {
	switch target {
	case Target_ActiveConfiguration:
		switch action {
		case Action_Set:
			res, err := ct.DryRunApplyConfig(ctx, spec)
			if err != nil {
				return nil, err
			}
			return &DryRunResults[T]{
				Current:  res.Current,
				Modified: res.Modified,
			}, nil
		case Action_Reset:
			res, err := ct.DryRunResetConfig(ctx)
			if err != nil {
				return nil, err
			}
			return &DryRunResults[T]{
				Current:  res.Current,
				Modified: res.Modified,
			}, nil
		default:
			return nil, fmt.Errorf("invalid action: %s", action)
		}
	case Target_DefaultConfiguration:
		switch action {
		case Action_Set:
			res, err := ct.DryRunSetDefaultConfig(ctx, spec)
			if err != nil {
				return nil, err
			}
			return &DryRunResults[T]{
				Current:  res.Current,
				Modified: res.Modified,
			}, nil
		case Action_Reset:
			res, err := ct.DryRunResetDefaultConfig(ctx)
			if err != nil {
				return nil, err
			}
			return &DryRunResults[T]{
				Current:  res.Current,
				Modified: res.Modified,
			}, nil
		default:
			return nil, fmt.Errorf("invalid action: %s", action)
		}
	default:
		return nil, fmt.Errorf("invalid target: %s", target)
	}
}

func (ct *DefaultingConfigTracker[T]) DryRunApplyConfig(ctx context.Context, newConfig T) (DryRunResults[T], error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	current, rev, err := ct.getConfigOrDefaultLocked(ctx)
	if err != nil {
		return DryRunResults[T]{}, err
	}

	if newConfig.GetRevision().GetRevision() != rev {
		return DryRunResults[T]{}, storage.ErrConflict
	}

	if err := newConfig.UnredactSecrets(current); err != nil {
		return DryRunResults[T]{}, err
	}

	modified := util.ProtoClone(current)

	merge.MergeWithReplace(modified, newConfig)

	current.RedactSecrets()
	modified.RedactSecrets()

	// Keep the old revision in the dry run results. Revisions are not stored
	// in the actual object inside the kv store, so they should not be included
	// in the diff.
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
	if newDefault.GetRevision().GetRevision() != rev {
		return DryRunResults[T]{}, storage.ErrConflict
	}

	if err := newDefault.UnredactSecrets(current); err != nil {
		return DryRunResults[T]{}, err
	}

	current.RedactSecrets()
	newDefault.RedactSecrets()

	return DryRunResults[T]{
		Current:  current,
		Modified: newDefault,
	}, nil
}

func (ct *DefaultingConfigTracker[T]) DryRunResetDefaultConfig(ctx context.Context) (DryRunResults[T], error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	current, err := ct.defaultStore.Get(ctx)
	if err != nil {
		// always return an error here, including not found errors, since
		// dry run reset only makes sense if there is an overridden default
		// config to reset
		return DryRunResults[T]{}, err
	}

	newDefault := ct.newDefaultSpec()

	current.RedactSecrets()
	newDefault.RedactSecrets()
	return DryRunResults[T]{
		Current:  current,
		Modified: newDefault,
	}, nil
}

func (ct *DefaultingConfigTracker[T]) DryRunResetConfig(ctx context.Context) (DryRunResults[T], error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	current, err := ct.activeStore.Get(ctx)
	if err != nil {
		return DryRunResults[T]{}, err
	}

	currentDefault, _, err := ct.getDefaultConfigLocked(ctx)
	if err != nil {
		return DryRunResults[T]{}, err
	}

	current.RedactSecrets()
	currentDefault.RedactSecrets()
	return DryRunResults[T]{
		Current:  current,
		Modified: currentDefault,
	}, nil
}

func (ct *DefaultingConfigTracker[T]) setRevision(t T, value int64) {
	if rev := t.GetRevision(); rev == nil {
		field := t.ProtoReflect().Descriptor().Fields().Get(ct.revisionFieldIndex)
		updatedRev := &corev1.Revision{Revision: &value}
		t.ProtoReflect().Set(field, protoreflect.ValueOfMessage(updatedRev.ProtoReflect()))
	} else {
		rev.Set(value)
	}
}

func (ct *DefaultingConfigTracker[T]) unsetRevision(t T) {
	if rev := t.GetRevision(); rev != nil {
		field := t.ProtoReflect().Descriptor().Fields().Get(ct.revisionFieldIndex)
		t.ProtoReflect().Clear(field)
	}
}

func getRevisionFieldIndex[T config_type[T]]() int {
	var revision corev1.Revision
	revisionFqn := revision.ProtoReflect().Descriptor().FullName()
	var t T
	fields := t.ProtoReflect().Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if field.Kind() == protoreflect.MessageKind {
			if field.Message().FullName() == revisionFqn {
				return i
			}
		}
	}
	panic("revision field not found")
}
