package otel

import (
	"context"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	// otlpcommonv1 "go.opentelemetry.io/proto/otlp/common/v1"
)

type OTELForwarder struct {
	util.Initializer

	colmetricspb.UnsafeMetricsServiceServer
	remoteTarget future.Future[colmetricspb.MetricsServiceClient]
	otelForwarderOptions
}

var _ colmetricspb.MetricsServiceServer = (*OTELForwarder)(nil)

type otelForwarderOptions struct {
	cc                grpc.ClientConnInterface
	targetDialOptions []grpc.DialOption
	remoteAddress     string
	logger            *zap.SugaredLogger
}

type OTELForwarderOption func(*otelForwarderOptions)

func (o *otelForwarderOptions) apply(opts ...OTELForwarderOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithClientConn(cc grpc.ClientConnInterface) OTELForwarderOption {
	return func(o *otelForwarderOptions) {
		o.cc = cc
	}
}

func WithDialOptions(opts ...grpc.DialOption) OTELForwarderOption {
	return func(o *otelForwarderOptions) {
		o.targetDialOptions = opts
	}
}

func WithLogger(lg *zap.SugaredLogger) OTELForwarderOption {
	return func(o *otelForwarderOptions) {
		o.logger = lg
	}
}

func WithRemoteAddress(addr string) OTELForwarderOption {
	return func(o *otelForwarderOptions) {
		o.remoteAddress = addr
	}
}

func NewOTELForwarder(ctx context.Context, opts ...OTELForwarderOption) *OTELForwarder {
	options := &otelForwarderOptions{
		logger: logger.NewPluginLogger().Named("metrics-otel-forwarder"),
	}
	options.apply(opts...)
	o := &OTELForwarder{
		otelForwarderOptions: *options,
		remoteTarget:         future.New[colmetricspb.MetricsServiceClient](),
	}
	go func() {
		o.Initialize(ctx)
	}()
	return o
}

func (o *OTELForwarder) Initialize(ctx context.Context) {
	o.InitOnce(func() {
		expBackoff := backoff.Exponential(
			backoff.WithMaxRetries(0),
			backoff.WithMinInterval(5*time.Second),
			backoff.WithMaxInterval(1*time.Minute),
			backoff.WithMultiplier(1.1),
		)
		b := expBackoff.Start(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-b.Next():
				conn, err := grpc.Dial(
					o.remoteAddress,
					o.targetDialOptions...,
				)
				if err != nil {
					o.logger.Warnf("failed to dial to remote target : %s", err)
					continue
				}
				o.remoteTarget.Set(colmetricspb.NewMetricsServiceClient(conn))
				return
			}
		}
	})
}

func (o *OTELForwarder) Export(
	ctx context.Context,
	req *colmetricspb.ExportMetricsServiceRequest,
) (*colmetricspb.ExportMetricsServiceResponse, error) {
	ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := o.WaitForInitContext(ctxca); err != nil {
		return nil, util.StatusError(codes.Unavailable)
	}

	return o.forwardMetricsToRemote(ctx, req)
}

func (o *OTELForwarder) forwardMetricsToRemote(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) (*colmetricspb.ExportMetricsServiceResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	remoteTarget, err := o.remoteTarget.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	return remoteTarget.Export(ctx, req)
}
