package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"

	"github.com/hashicorp/go-plugin"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/kralicky/totem"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type GatewayStreamApiExtensionPluginOptions struct {
	metricsConfig GatewayStreamMetricsConfig
}

type GatewayStreamApiExtensionPluginOption func(*GatewayStreamApiExtensionPluginOptions)

func (o *GatewayStreamApiExtensionPluginOptions) apply(opts ...GatewayStreamApiExtensionPluginOption) {
	for _, op := range opts {
		op(o)
	}
}

type GatewayStreamMetricsConfig struct {
	// Prometheus registerer
	Reader metric.Reader

	// A function called on each stream's Connect that returns a list of static
	// labels to attach to all metrics collected for that stream.
	LabelsForStream func(context.Context) []attribute.KeyValue
}

func WithMetrics(conf GatewayStreamMetricsConfig) GatewayStreamApiExtensionPluginOption {
	return func(o *GatewayStreamApiExtensionPluginOptions) {
		o.metricsConfig = conf
	}
}

func NewGatewayPlugin(ctx context.Context, p StreamAPIExtension, opts ...GatewayStreamApiExtensionPluginOption) plugin.Plugin {
	options := GatewayStreamApiExtensionPluginOptions{}
	options.apply(opts...)

	pc, _, _, ok := runtime.Caller(1)
	fn := runtime.FuncForPC(pc)
	name := "unknown"
	if ok {
		fnName := fn.Name()
		parts := strings.Split(fnName, "/")
		name = fmt.Sprintf("plugin_%s", parts[slices.Index(parts, "plugins")+1])
	}

	lg := logger.NewPluginLogger(ctx).WithGroup(name).WithGroup("stream")
	ctx = logger.WithPluginLogger(ctx, lg)
	ext := &gatewayStreamExtensionServerImpl{
		ctx:           ctx,
		name:          name,
		metricsConfig: options.metricsConfig,
	}
	if p != nil {
		if options.metricsConfig.Reader != nil {
			ext.meterProvider = metric.NewMeterProvider(metric.WithReader(options.metricsConfig.Reader),
				metric.WithResource(resource.NewSchemaless(
					attribute.Key("plugin").String(name),
					attribute.String("system", "opni_gateway"),
				)),
			)
		}
		servers := p.StreamServers()
		for _, srv := range servers {
			descriptor, err := grpcreflect.LoadServiceDescriptor(srv.Desc)
			if err != nil {
				panic(err)
			}
			ext.servers = append(ext.servers, &richServer{
				Server:   srv,
				richDesc: descriptor,
			})
		}
		if clientHandler, ok := p.(StreamClientHandler); ok {
			ext.clientHandler = clientHandler
		}
	}
	return &streamApiExtensionPlugin[*gatewayStreamExtensionServerImpl]{
		extensionSrv: ext,
	}
}

type gatewayStreamExtensionServerImpl struct {
	streamv1.UnimplementedStreamServer
	apiextensions.UnsafeStreamAPIExtensionServer

	ctx           context.Context
	name          string
	servers       []*richServer
	clientHandler StreamClientHandler
	metricsConfig GatewayStreamMetricsConfig
	meterProvider *metric.MeterProvider
}

// Implements streamv1.StreamServer
func (e *gatewayStreamExtensionServerImpl) Connect(stream streamv1.Stream_ConnectServer) error {
	lg := logger.PluginLoggerFromContext(e.ctx)
	id := cluster.StreamAuthorizedID(stream.Context())

	lg.With(
		"id", id,
	).Debug("stream connected")

	opts := []totem.ServerOption{
		totem.WithName("gateway-apiext"),
		totem.WithTracerOptions(
			resource.WithAttributes(
				semconv.ServiceNameKey.String(e.name),
				attribute.String("mode", "gateway"),
				attribute.String("agent", id),
			),
		),
	}

	if e.meterProvider != nil {
		var labels []attribute.KeyValue
		if e.metricsConfig.LabelsForStream != nil {
			labels = e.metricsConfig.LabelsForStream(stream.Context())
		}

		opts = append(opts, totem.WithMetrics(e.meterProvider, labels...))
	}

	ts, err := totem.NewServer(stream, opts...)

	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to create stream server")
		return err
	}
	for _, srv := range e.servers {
		ts.RegisterService(srv.Desc, srv.Impl)
	}

	_, errC := ts.Serve()

	lg.Debug("stream server started")

	err = <-errC
	if errors.Is(err, io.EOF) || status.Code(err) == codes.OK {
		lg.Debug("stream server exited")
	} else if status.Code(err) == codes.Canceled {
		lg.Debug("stream server closed")
	} else {
		lg.With(
			logger.Err(err),
		).Warn("stream server exited with error")
	}
	return err
}

// ConnectInternal implements apiextensions.StreamAPIExtensionServer
func (e *gatewayStreamExtensionServerImpl) ConnectInternal(stream apiextensions.StreamAPIExtension_ConnectInternalServer) error {
	lg := logger.PluginLoggerFromContext(e.ctx)

	if e.clientHandler == nil {
		stream.SendHeader(metadata.Pairs("accept-internal-stream", "false"))
		return nil
	}
	stream.SendHeader(metadata.Pairs("accept-internal-stream", "true"))

	lg.Debug("internal gateway stream connected")

	ts, err := totem.NewServer(
		stream,
		totem.WithName("gateway-internal-client"),
		totem.WithTracerOptions(
			resource.WithAttributes(
				semconv.ServiceNameKey.String("gateway-internal-client"),
				semconv.ServiceInstanceIDKey.String(e.name),
			),
		),
	)
	if err != nil {
		return err
	}
	cc, errC := ts.Serve()
	select {
	case err := <-errC:
		if errors.Is(err, io.EOF) {
			lg.Debug("stream disconnected")
		} else if status.Code(err) == codes.Canceled {
			lg.Debug("stream closed")
		} else {
			lg.With(
				logger.Err(err),
			).Warn("stream disconnected with error")
		}
		return err
	default:
	}

	lg.Debug("calling client handler")
	go e.clientHandler.UseStreamClient(cc)

	return <-errC
}
