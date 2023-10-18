package driverutil

import "context"

type ClientContextInjector[T any] interface {
	ContextWithClient(context.Context, T) context.Context
	ClientFromContext(context.Context) (T, bool)
}
