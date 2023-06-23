package targets

import (
	"context"
)

type (
	NoopTracer struct{}
	NoopSpan   struct{}
)

func (NoopSpan) End() {}

func (NoopTracer) Start(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, NoopSpan{}
}

type TracerInterface interface {
	Start(ctx context.Context, name string) (context.Context, Span)
}

type Span interface {
	End()
}

var Tracer TracerInterface = NoopTracer{}
