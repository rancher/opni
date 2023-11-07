package driverutil

import (
	"context"

	"google.golang.org/grpc"
)

type ClientContextInjector[T any] interface {
	NewClient(grpc.ClientConnInterface) T
	UnderlyingConn(T) grpc.ClientConnInterface
	ContextWithClient(context.Context, T) context.Context
	ClientFromContext(context.Context) (T, bool)
}
