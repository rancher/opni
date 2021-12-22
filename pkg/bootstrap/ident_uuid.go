package bootstrap

import (
	"context"

	"github.com/google/uuid"
)

type uuidIdentProvider struct{}

// Returns an IdentProvider that generates a random UUID each time.
func NewUUIDIdentProvider() IdentProvider {
	return &uuidIdentProvider{}
}

func (p *uuidIdentProvider) UniqueIdentifier(ctx context.Context) string {
	return uuid.NewString()
}
