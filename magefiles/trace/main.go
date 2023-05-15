package main

import (
	"context"
	"fmt"
	"os"

	"github.com/magefile/mage/mage"
	"github.com/rancher/opni/magefiles/targets"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

func main() {
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize jaeger exporter: %v\n", err)
		os.Exit(1)
	}
	tp := tracesdk.NewTracerProvider(tracesdk.WithBatcher(exp),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("mage"),
		)))
	targets.Tracer = otelTracerWrapper{tp.Tracer("mage")}

	exitCode := mage.Main()

	tp.ForceFlush(context.Background())
	os.Exit(exitCode)
}

type otelTracerWrapper struct {
	trace.Tracer
}

func (t otelTracerWrapper) Start(ctx context.Context, name string) (context.Context, targets.Span) {
	ctx, span := t.Tracer.Start(ctx, name)
	return ctx, otelSpanWrapper{span}
}

type otelSpanWrapper struct {
	trace.Span
}

func (s otelSpanWrapper) End() {
	s.Span.End()
}
