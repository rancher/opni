package ident

import "context"

type Provider interface {
	UniqueIdentifier(ctx context.Context) string
}
