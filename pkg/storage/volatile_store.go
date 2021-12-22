package storage

import (
	"context"
	"errors"
	"sync"

	"github.com/kralicky/opni-gateway/pkg/tokens"
)

var ErrTokenNotFound = errors.New("token not found")

type VolatileTokenStore struct {
	cache tokenCache
}

var _ TokenStore = (*VolatileTokenStore)(nil)

func NewVolatileTokenStore() TokenStore {
	return &VolatileTokenStore{
		cache: tokenCache{
			Mutex: &sync.Mutex{},
			data:  make(map[string]string),
		},
	}
}

func (v *VolatileTokenStore) CreateToken(_ context.Context) (*tokens.Token, error) {
	token := tokens.NewToken()
	v.cache.Lock()
	defer v.cache.Unlock()
	v.cache.data[token.HexID()] = token.EncodeHex()
	return token, nil
}

func (v *VolatileTokenStore) DeleteToken(_ context.Context, tokenID string) error {
	v.cache.Lock()
	defer v.cache.Unlock()
	delete(v.cache.data, tokenID)
	return nil
}

func (v *VolatileTokenStore) TokenExists(_ context.Context, tokenID string) (bool, error) {
	v.cache.Lock()
	defer v.cache.Unlock()
	_, ok := v.cache.data[tokenID]
	return ok, nil
}

func (v *VolatileTokenStore) GetToken(_ context.Context, tokenID string) (*tokens.Token, error) {
	v.cache.Lock()
	defer v.cache.Unlock()
	secret, ok := v.cache.data[tokenID]
	if !ok {
		return nil, ErrTokenNotFound
	}
	return tokens.DecodeHexToken(secret)
}

func (v *VolatileTokenStore) ListTokens(_ context.Context) ([]string, error) {
	v.cache.Lock()
	defer v.cache.Unlock()
	tokens := make([]string, 0, len(v.cache.data))
	for tokenID := range v.cache.data {
		tokens = append(tokens, tokenID)
	}
	return tokens, nil
}
