package bootstrap

import "context"

type IdentProvider interface {
	UniqueIdentifier(ctx context.Context) string
}
