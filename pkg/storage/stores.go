package storage

import (
	"context"
	"errors"
	"time"

	"github.com/kralicky/opni-gateway/pkg/keyring"
	"github.com/kralicky/opni-gateway/pkg/tokens"
)

var ErrNotFound = errors.New("not found")

type TokenStore interface {
	CreateToken(ctx context.Context, ttl time.Duration) (*tokens.Token, error)
	DeleteToken(ctx context.Context, tokenID string) error
	TokenExists(ctx context.Context, tokenID string) (bool, error)
	GetToken(ctx context.Context, tokenID string) (*tokens.Token, error)
	ListTokens(ctx context.Context) ([]*tokens.Token, error)
}

type TenantStore interface {
	CreateTenant(ctx context.Context, tenantID string) error
	DeleteTenant(ctx context.Context, tenantID string) error
	TenantExists(ctx context.Context, tenantID string) (bool, error)
	ListTenants(ctx context.Context) ([]string, error)
	KeyringStore(ctx context.Context, tenantID string) (KeyringStore, error)
}

type KeyringStore interface {
	Put(ctx context.Context, keyring keyring.Keyring) error
	Get(ctx context.Context) (keyring.Keyring, error)
}
