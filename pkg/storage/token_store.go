package storage

import (
	"context"
	"errors"
	"time"

	"github.com/kralicky/opni-gateway/pkg/tokens"
)

var ErrTokenNotFound = errors.New("token not found")

type TokenStore interface {
	CreateToken(ctx context.Context, ttl time.Duration) (*tokens.Token, error)
	DeleteToken(ctx context.Context, tokenID string) error
	TokenExists(ctx context.Context, tokenID string) (bool, error)
	GetToken(ctx context.Context, tokenID string) (*tokens.Token, error)
	ListTokens(ctx context.Context) ([]*tokens.Token, error)
}
