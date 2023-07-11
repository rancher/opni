package driverutil

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/rancher/opni/pkg/storage"
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

type DefaultingConfigTracker[S config_type[S]] struct {
	lock          sync.Mutex
	defaultStore  storage.ValueStoreT[S]
	activeStore   storage.ValueStoreT[S]
	defaultLoader DefaultLoaderFunc[S]
}

func NewDefaultingConfigTracker[S config_type[S]](
	defaultStore, activeStore storage.ValueStoreT[S],
	loadDefaultsFunc DefaultLoaderFunc[S],
) *DefaultingConfigTracker[S] {
	return &DefaultingConfigTracker[S]{
		defaultStore:  defaultStore,
		activeStore:   activeStore,
		defaultLoader: loadDefaultsFunc,
	}
}

func (ct *DefaultingConfigTracker[S]) newDefaultSpec() (s S) {
	s = s.ProtoReflect().New().Interface().(S)
	ct.defaultLoader(s)
	return s
}

// Gets the default config if one has been set, otherwise returns a new default
// config as defined by the type.
func (ct *DefaultingConfigTracker[S]) GetDefaultConfig(ctx context.Context) (S, error) {
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
func (ct *DefaultingConfigTracker[S]) SetDefaultConfig(ctx context.Context, newDefault S) error {
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
func (ct *DefaultingConfigTracker[S]) ResetDefaultConfig(ctx context.Context) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	if err := ct.defaultStore.Delete(ctx); err != nil {
		return fmt.Errorf("error resetting config: %w", err)
	}
	return nil
}

func (ct *DefaultingConfigTracker[S]) getDefaultConfigLocked(ctx context.Context) (S, error) {
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
func (ct *DefaultingConfigTracker[S]) GetConfig(ctx context.Context) (S, error) {
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
func (ct *DefaultingConfigTracker[S]) ResetConfig(ctx context.Context) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	if err := ct.activeStore.Delete(ctx); err != nil {
		return fmt.Errorf("error restoring config: %w", err)
	}
	return nil
}

// Returns the active config if it has been set, otherwise returns the default config.
func (ct *DefaultingConfigTracker[S]) GetConfigOrDefault(ctx context.Context) (S, error) {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	value, err := ct.getConfigOrDefaultLocked(ctx)
	if err != nil {
		return value, err
	}

	value.RedactSecrets()
	return value, nil
}

func (ct *DefaultingConfigTracker[S]) getConfigOrDefaultLocked(ctx context.Context) (S, error) {
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

// Apply sets the active config by merging the given config onto the existing
// active config, or onto the default config if no active config has been set.
func (ct *DefaultingConfigTracker[S]) ApplyConfig(ctx context.Context, newConfig S) error {
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

// SetConfig sets the active config by mergeing the given config onto the default
// config, regardless of whether an active config has been set.
func (ct *DefaultingConfigTracker[S]) SetConfig(ctx context.Context, newConfig S) error {
	ct.lock.Lock()
	defer ct.lock.Unlock()

	def, err := ct.getDefaultConfigLocked(ctx)
	if err != nil {
		return err
	}

	merge.MergeWithReplace(def, newConfig)
	return ct.activeStore.Put(ctx, def)
}
