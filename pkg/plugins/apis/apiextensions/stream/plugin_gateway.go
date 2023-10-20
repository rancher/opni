package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"

	"log/slog"

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

func NewGatewayPlugin(p StreamAPIExtension, opts ...GatewayStreamApiExtensionPluginOption) plugin.Plugin {
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

	ext := &gatewayStreamExtensionServerImpl{
		name:          name,
		logger:        logger.NewPluginLogger().WithGroup(name).WithGroup("stream"),
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

	name          string
	servers       []*richServer
	clientHandler StreamClientHandler
	logger        *slog.Logger
	metricsConfig GatewayStreamMetricsConfig
	meterProvider *metric.MeterProvider
}

// Implements streamv1.StreamServer
func (e *gatewayStreamExtensionServerImpl) Connect(stream streamv1.Stream_ConnectServer) error {
	id := cluster.StreamAuthorizedID(stream.Context())

	e.logger.With(
		zap.String("id", id),
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
		e.logger.With(
			zap.Error(err),
		).Error("failed to create stream server")
		return err
	}
	for _, srv := range e.servers {
		ts.RegisterService(srv.Desc, srv.Impl)
	}

	_, errC := ts.Serve()

	e.logger.Debug("stream server started")

	err = <-errC
	if errors.Is(err, io.EOF) || status.Code(err) == codes.OK {
		e.logger.Debug("stream server exited")
	} else if status.Code(err) == codes.Canceled {
		e.logger.Debug("stream server closed")
	} else {
		e.logger.With(
			zap.Error(err),
		).Warn("stream server exited with error")
	}
	return err
}

// ConnectInternal implements apiextensions.StreamAPIExtensionServer
func (e *gatewayStreamExtensionServerImpl) ConnectInternal(stream apiextensions.StreamAPIExtension_ConnectInternalServer) error {
	if e.clientHandler == nil {
		stream.SendHeader(metadata.Pairs("accept-internal-stream", "false"))
		return nil
	}
	stream.SendHeader(metadata.Pairs("accept-internal-stream", "true"))

	e.logger.Debug("internal gateway stream connected")

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
			e.logger.Debug("stream disconnected")
		} else if status.Code(err) == codes.Canceled {
			e.logger.Debug("stream closed")
		} else {
			e.logger.With(
				zap.Error(err),
			).Warn("stream disconnected with error")
		}
		return err
	default:
	}

	e.logger.Debug("calling client handler")
	go e.clientHandler.UseStreamClient(cc)

	return <-errC
}
