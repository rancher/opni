package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/pkg/util/future"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	opniPreprocessingAddress = "opni-preprocess-otel"
	opniPreprocessingPort    = 4317
)

type OTELForwarder struct {
	client future.Future[collogspb.LogsServiceClient]
	logger *zap.SugaredLogger
}

type otelForwarderOptions struct {
	collectorAddressOverride string
}

type OTELForwarderOption func(*otelForwarderOptions)

func (o *otelForwarderOptions) apply(opts ...OTELForwarderOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithAddress(address string) OTELForwarderOption {
	return func(o *otelForwarderOptions) {
		o.collectorAddressOverride = address
	}
}

func (f *OTELForwarder) InitializeOTELForwarder(opts ...OTELForwarderOption) {
	options := otelForwarderOptions{
		collectorAddressOverride: fmt.Sprintf("http://%s:%d", opniPreprocessingAddress, opniPreprocessingPort),
	}
	options.apply(opts...)

	ctx := context.Background()
	expBackoff := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(5*time.Second),
		backoff.WithMaxInterval(1*time.Minute),
		backoff.WithMultiplier(1.1),
	)
	b := expBackoff.Start(ctx)

CONNECT:
	for {
		select {
		case <-b.Done():
			f.logger.Warn("plugin context cancelled before gRPC client created")
			return
		case <-b.Next():
			conn, err := grpc.Dial(options.collectorAddressOverride)
			if err != nil {
				f.logger.Errorf("failed dial grpc: %v", err)
				continue
			}
			f.client.Set(collogspb.NewLogsServiceClient(conn))
			break CONNECT
		}
	}
}

func (f *OTELForwarder) Export(
	ctx context.Context,
	request *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	if f.client.IsSet() {
		return f.client.Get().Export(ctx, request)
	}
	return nil, status.Errorf(codes.Unavailable, "collector is unavailable")
}
