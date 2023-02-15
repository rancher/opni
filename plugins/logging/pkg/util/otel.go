package util

import (
	"context"
	"time"

	"github.com/lestrrat-go/backoff/v2"
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
	collogspb.UnsafeLogsServiceServer
	opts otelForwarderOptions

	Client *AsyncClient[collogspb.LogsServiceClient]
}

type otelForwarderOptions struct {
	collectorAddressOverride string
	cc                       grpc.ClientConnInterface
	lg                       *zap.SugaredLogger
	dialOptions              []grpc.DialOption
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

func WithLogger(lg *zap.SugaredLogger) OTELForwarderOption {
	return func(o *otelForwarderOptions) {
		o.lg = lg
	}
}

func WithDialOptions(opts ...grpc.DialOption) OTELForwarderOption {
	return func(o *otelForwarderOptions) {
		o.dialOptions = opts
	}
}

func NewOTELForwarder(opts ...OTELForwarderOption) *OTELForwarder {
	options := otelForwarderOptions{
		collectorAddressOverride: defaultAddress,
	}
	options.apply(opts...)
	return &OTELForwarder{
		opts:   options,
		Client: NewAsyncClient[collogspb.LogsServiceClient](),
	}
}

func (f *OTELForwarder) BackgroundInitClient() {
	f.Client.BackgroundInitClient(f.initializeOTELForwarder)
}

func (f *OTELForwarder) SetClient(cc grpc.ClientConnInterface) {
	client := collogspb.NewLogsServiceClient(cc)
	f.Client.SetClient(client)
}

func (f *OTELForwarder) initializeOTELForwarder() collogspb.LogsServiceClient {
	if f.opts.cc == nil {
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
				f.opts.lg.Warn("plugin context cancelled before gRPC client created")
				return nil
			case <-b.Next():
				opts := append(f.opts.dialOptions, grpc.WithBlock())
				conn, err := grpc.Dial(
					f.opts.collectorAddressOverride,
					opts...,
				)
				if err != nil {
					f.opts.lg.Errorf("failed dial grpc: %v", err)
					continue
				}
				return collogspb.NewLogsServiceClient(conn)
			}
		}
	}
	return collogspb.NewLogsServiceClient(f.opts.cc)
}

func (f *OTELForwarder) Export(
	ctx context.Context,
	request *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	if f.Client.IsSet() {
		return f.Client.Client.Export(ctx, request)
	}
	return nil, status.Errorf(codes.Unavailable, "collector is unavailable")
}
