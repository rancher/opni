package util

import (
	"context"
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
	defaultAddress = "http://localhost:8080"
)

type OTELForwarder struct {
	Client future.Future[collogspb.LogsServiceClient]
	Logger *zap.SugaredLogger
}

type otelForwarderOptions struct {
	collectorAddressOverride string
	cc                       grpc.ClientConnInterface
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

func WithClientConn(cc grpc.ClientConnInterface) OTELForwarderOption {
	return func(o *otelForwarderOptions) {
		o.cc = cc
	}
}

func (f *OTELForwarder) InitializeOTELForwarder(opts ...OTELForwarderOption) {
	options := otelForwarderOptions{
		collectorAddressOverride: defaultAddress,
	}
	options.apply(opts...)

	if options.cc == nil {
		ctx := context.Background()
		expBackoff := backoff.Exponential(
			backoff.WithMaxRetries(0),
			backoff.WithMinInterval(5*time.Second),
			backoff.WithMaxInterval(1*time.Minute),
			backoff.WithMultiplier(1.1),
		)
		b := expBackoff.Start(ctx)

		for {
			select {
			case <-b.Done():
				f.Logger.Warn("plugin context cancelled before gRPC client created")
				return
			case <-b.Next():
				conn, err := grpc.Dial(
					options.collectorAddressOverride,
					grpc.WithBlock(),
				)
				if err != nil {
					f.Logger.Errorf("failed dial grpc: %v", err)
					continue
				}
				f.Client.Set(collogspb.NewLogsServiceClient(conn))
				return
			}
		}
	}
	f.Client.Set(collogspb.NewLogsServiceClient(options.cc))
}

func (f *OTELForwarder) Export(
	ctx context.Context,
	request *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	if f.Client.IsSet() {
		return f.Client.Get().Export(ctx, request)
	}
	return nil, status.Errorf(codes.Unavailable, "collector is unavailable")
}
