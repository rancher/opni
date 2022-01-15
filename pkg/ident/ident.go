package ident

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Provider interface {
	UniqueIdentifier(ctx context.Context) (string, error)
}

type NamedProvider interface {
	Provider
	Name() string
}

type namedProviderImpl struct {
	Provider
	name string
}

func (np *namedProviderImpl) Name() string {
	return np.name
}

func namedProvider(name string, provider Provider) NamedProvider {
	return &namedProviderImpl{
		Provider: provider,
		name:     name,
	}
}

var (
	identProviders           = make(map[string]func() Provider)
	ErrInvalidProviderName   = errors.New("invalid or empty ident provider name")
	ErrProviderAlreadyExists = errors.New("ident provider already exists")
	ErrNilProvider           = errors.New("ident provider is nil")
	ErrProviderNotFound      = errors.New("ident provider not found")
)

func RegisterProvider(name string, provider func() Provider) error {
	name = strings.TrimSpace(name)
	if len(name) == 0 {
		return ErrInvalidProviderName
	}
	if _, ok := identProviders[name]; ok {
		return fmt.Errorf("%w: %s", ErrProviderAlreadyExists, name)
	}
	if provider == nil {
		return ErrNilProvider
	}
	identProviders[name] = provider
	return nil
}

func GetProvider(name string) (NamedProvider, error) {
	if p, ok := identProviders[name]; ok {
		return namedProvider(name, p()), nil
	}
	return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
}
