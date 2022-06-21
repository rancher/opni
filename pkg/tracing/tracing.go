package tracing

import (
	"context"

	"github.com/go-logr/zapr"
	"github.com/rancher/opni/pkg/logger"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
)

var log = logger.New().Named("tracing")

func Configure(serviceName string) {
	res, err := resource.New(context.Background(),
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		panic(err)
	}
	opts := []tracesdk.TracerProviderOption{
		tracesdk.WithResource(res),
	}
	otel.SetTracerProvider(tracesdk.NewTracerProvider(opts...))
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())
	otel.SetLogger(zapr.NewLogger(log.Desugar()))
}
