package driverutil

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/merge"
	"google.golang.org/protobuf/proto"
)

type DefaultLoaderFunc[T any] func(T)

type secretsRedactor[T any] interface {
	RedactSecrets()
	UnredactSecrets(T) error
}

type config_type[T any] interface {
	proto.Message
	secretsRedactor[T]
}

type DryRunResults[T any] struct {
	Current  T
	Modified T
}

type DefaultingConfigTracker[T config_type[T]] struct {
	lock          sync.Mutex
	defaultStore  storage.ValueStoreT[T]
	activeStore   storage.ValueStoreT[T]
	defaultLoader DefaultLoaderFunc[T]
}

func NewDefaultingConfigTracker[T config_type[T]](
	defaultStore, activeStore storage.ValueStoreT[T],
	loadDefaultsFunc DefaultLoaderFunc[T],
) *DefaultingConfigTracker[T] {
	return &DefaultingConfigTracker[T]{
		defaultStore:  defaultStore,
		activeStore:   activeStore,
		defaultLoader: loadDefaultsFunc,
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

	def, err := ct.getDefaultConfigLocked(ctx)
	if err != nil {
		return def, err
	}

	def.RedactSecrets()
	return def, nil
}

// Sets the default config directly. No merging is performed.
func (ct *DefaultingConfigTracker[T]) SetDefaultConfig(ctx context.Context, newDefault T) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	existing, err := ct.getDefaultConfigLocked(ctx)
	if err != nil {
		return err
	}
	if err := newDefault.UnredactSecrets(existing); err != nil {
		return err
	}

	return ct.defaultStore.Put(ctx, newDefault)
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

func (ct *DefaultingConfigTracker[T]) getDefaultConfigLocked(ctx context.Context) (T, error) {
	def, err := ct.defaultStore.Get(ctx)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return def, fmt.Errorf("error looking up default config: %w", err)
		}
		def = ct.newDefaultSpec()
	}
	return def, nil
}

// Returns the active config if it has been set, otherwise returns a "not found" error.
func (ct *DefaultingConfigTracker[T]) GetConfig(ctx context.Context) (T, error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	existing, err := ct.activeStore.Get(ctx)
	if err != nil {
		return existing, fmt.Errorf("error looking up config: %w", err)
	}
	existing.RedactSecrets()
	return existing, nil
}

// Deletes the active config, leaving the default config as-is. Subsequent calls
// to GetConfigOrDefault will return the default config.
func (ct *DefaultingConfigTracker[T]) ResetConfig(ctx context.Context) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	if err := ct.activeStore.Delete(ctx); err != nil {
		return fmt.Errorf("error restoring config: %w", err)
	}
	return nil
}

// Returns the active config if it has been set, otherwise returns the default config.
func (ct *DefaultingConfigTracker[T]) GetConfigOrDefault(ctx context.Context) (T, error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	value, err := ct.getConfigOrDefaultLocked(ctx)
	if err != nil {
		return value, err
	}

	value.RedactSecrets()
	return value, nil
}

func (ct *DefaultingConfigTracker[T]) getConfigOrDefaultLocked(ctx context.Context) (T, error) {
	value, err := ct.activeStore.Get(ctx)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return value, fmt.Errorf("error looking up config: %w", err)
		}
		value, err = ct.defaultStore.Get(ctx)
		if err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				return value, fmt.Errorf("error looking up default config: %w", err)
			}
			value = ct.newDefaultSpec()
		}
	}
	return value, nil
}

// ApplyConfig sets the active config by merging the given config onto the existing
// active config, or onto the default config if no active config has been set.
func (ct *DefaultingConfigTracker[T]) ApplyConfig(ctx context.Context, newConfig T) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	existing, err := ct.getConfigOrDefaultLocked(ctx)
	if err != nil {
		return err
	}
	if err := newConfig.UnredactSecrets(existing); err != nil {
		return err
	}

	merge.MergeWithReplace(existing, newConfig)
	return ct.activeStore.Put(ctx, existing)
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
	current, err := ct.getConfigOrDefaultLocked(ctx)
	if err != nil {
		return DryRunResults[T]{}, err
	}
	if err := newConfig.UnredactSecrets(current); err != nil {
		return DryRunResults[T]{}, err
	}

	modified := util.ProtoClone(current)

	merge.MergeWithReplace(modified, newConfig)

	current.RedactSecrets()
	modified.RedactSecrets()
	return DryRunResults[T]{
		Current:  current,
		Modified: modified,
	}, nil
}

func (ct *DefaultingConfigTracker[T]) DryRunSetDefaultConfig(ctx context.Context, newDefault T) (DryRunResults[T], error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()
	current, err := ct.getDefaultConfigLocked(ctx)
	if err != nil {
		return DryRunResults[T]{}, err
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

	currentDefault, err := ct.getDefaultConfigLocked(ctx)
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
