package bootstrap

import (
	"context"
	"errors"
	"sync"
)

var ErrTokenNotFound = errors.New("token not found")

type VolatileTokenStore struct {
	cache tokenCache
}

var _ TokenStore = (*VolatileTokenStore)(nil)

func NewVolatileSecretStore() TokenStore {
	return &VolatileTokenStore{
		cache: tokenCache{
			Mutex: &sync.Mutex{},
			data:  make(map[string]string),
		},
	}
}

func (v *VolatileTokenStore) CreateToken(_ context.Context) (*Token, error) {
	token := NewToken()
	v.cache.Lock()
	defer v.cache.Unlock()
	v.cache.data[token.HexID()] = string(token.Secret)
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

func (v *VolatileTokenStore) GetToken(_ context.Context, tokenID string) (*Token, error) {
	v.cache.Lock()
	defer v.cache.Unlock()
	secret, ok := v.cache.data[tokenID]
	if !ok {
		return nil, ErrTokenNotFound
	}
	return &Token{
		ID:     []byte(tokenID),
		Secret: []byte(secret),
	}, nil
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
