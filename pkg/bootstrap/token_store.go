package bootstrap

import (
	"context"
	"sync"
)

type TokenStore interface {
	CreateToken(ctx context.Context) (*Token, error)
	DeleteToken(ctx context.Context, tokenID string) error
	TokenExists(ctx context.Context, tokenID string) (bool, error)
	GetToken(ctx context.Context, tokenID string) (*Token, error)
	ListTokens(ctx context.Context) ([]string, error)
}

type tokenCache struct {
	*sync.Mutex
	data map[string]string
}
