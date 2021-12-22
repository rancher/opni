package storage

import (
	"context"
	"sync"

	"github.com/kralicky/opni-gateway/pkg/tokens"
)

type TokenStore interface {
	CreateToken(ctx context.Context) (*tokens.Token, error)
	DeleteToken(ctx context.Context, tokenID string) error
	TokenExists(ctx context.Context, tokenID string) (bool, error)
	GetToken(ctx context.Context, tokenID string) (*tokens.Token, error)
	ListTokens(ctx context.Context) ([]string, error)
}

type tokenCache struct {
	*sync.Mutex
	data map[string]string
}
