package otel

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/otel"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	otlpcommonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	metricsTenantId = "__tenant_id__"
)

type otelInitializerOptions struct {
	cc                grpc.ClientConnInterface
	targetDialOptions []grpc.DialOption
	remoteAddress     string
	logger            *zap.SugaredLogger
}

type OTELInitializerOption func(*otelInitializerOptions)

func (o *otelInitializerOptions) apply(opts ...OTELInitializerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithClientConn(cc grpc.ClientConnInterface) OTELInitializerOption {
	return func(o *otelInitializerOptions) {
		o.cc = cc
	}
}

func WithDialOptions(opts ...grpc.DialOption) OTELInitializerOption {
	return func(o *otelInitializerOptions) {
		o.targetDialOptions = opts
	}
}

func WithLogger(lg *zap.SugaredLogger) OTELInitializerOption {
	return func(o *otelInitializerOptions) {
		o.logger = lg
	}
}

func WithRemoteAddress(addr string) OTELInitializerOption {
	return func(o *otelInitializerOptions) {
		o.remoteAddress = addr
	}
}

type otelInitializer struct {
	util.Initializer

	overrideClientSignal chan colmetricspb.MetricsServiceClient
	otelInitializerOptions
	remoteTarget future.Future[colmetricspb.MetricsServiceClient]
}

func NewOtelInitializer(opts ...OTELInitializerOption) *otelInitializer {
	options := &otelInitializerOptions{
		logger: logger.NewPluginLogger().Named("metrics-otel-intializer"),
	}
	options.apply(opts...)
	o := &otelInitializer{
		otelInitializerOptions: *options,
		overrideClientSignal:   make(chan colmetricspb.MetricsServiceClient, 1),
		remoteTarget:           future.New[colmetricspb.MetricsServiceClient](),
	}
	return o
}

func (o *otelInitializer) SetClient(client colmetricspb.MetricsServiceClient) {
	defer close(o.overrideClientSignal)
	o.overrideClientSignal <- client
}

func (o *otelInitializer) Initialize(ctx context.Context) {
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
				if o.remoteAddress == "" {
					continue
				}
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
			case client := <-o.overrideClientSignal:
				o.remoteTarget.Set(client)
				return
			}
		}
	})
}

type OTELForwarder struct {
	colmetricspb.UnsafeMetricsServiceServer

	logger *zap.SugaredLogger

	otelInitializer
}

var _ colmetricspb.MetricsServiceServer = (*OTELForwarder)(nil)

// NewOTELForwarder creates a new OTEL forwarder instance.
// If a remote address is omitted, the inner OTEL initializer will expect
// the SetClient method to be called on the forwarder instance to
// acquire the remote target address.
func NewOTELForwarder(ctx context.Context, opts ...OTELInitializerOption) *OTELForwarder {
	o := &OTELForwarder{
		logger:          logger.NewPluginLogger().Named("metrics-otel-forwarder"),
		otelInitializer: *NewOtelInitializer(opts...),
	}
	go func() {
		o.Initialize(ctx)
	}()
	return o
}

func (o *OTELForwarder) Export(
	ctx context.Context,
	req *colmetricspb.ExportMetricsServiceRequest,
) (*colmetricspb.ExportMetricsServiceResponse, error) {
	ctxca, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := o.WaitForInitContext(ctxca); err != nil {
		return nil, status.Errorf(codes.Unavailable, "collector is unavailable")
	}
	// tenantId := cluster.StreamAuthorizedID(ctx)

	// TODO : want to annotate with private __tenant_id__

	// TODO : want to drop data points with certain labels

	for _, resourceMetrics := range req.GetResourceMetrics() {
		// undecided whether or not we should include this safeguard
		if resourceMetrics == nil {
			continue
		}
		// Shared scope attributes for metrics will convert to metric labels as appropriate when exported :
		// https://github.com/open-telemetry/oteps/blob/main/text/0201-scope-attributes.md#exporting-to-non-otlp
		for _, scopedMetrics := range resourceMetrics.GetScopeMetrics() {
			if scopedMetrics == nil {
				continue
			}
			// if !clusterIdExists(scopedMetrics.Scope.Attributes) {
			// 	scopedMetrics.Scope.Attributes = append(
			// 		scopedMetrics.Scope.Attributes,
			// 		&otlpcommonv1.KeyValue{
			// 			Key: metricsTenantId,
			// 			Value: &otlpcommonv1.AnyValue{
			// 				Value: &otlpcommonv1.AnyValue_StringValue{
			// 					StringValue: tenantId,
			// 				},
			// 			},
			// 		},
			// 	)
			// }
		}
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

func clusterIdExists(attrs []*otlpcommonv1.KeyValue) bool {
	for _, kv := range attrs {
		if kv.GetKey() == metricsTenantId {
			return true
		}
	}
	return false
}

func standardizeSourceLabels() {
	//TODO
}

func (f *OTELForwarder) handleMetricsPost(c *gin.Context) {
	if !f.Initialized() {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	switch c.ContentType() {
	case otel.PbContentType:
		f.renderProto(c)
	case otel.JsonContentType:
		f.renderProtoJSON(c)
	default:
		c.String(http.StatusUnsupportedMediaType, "unsupported media type, supported: [%s,%s]", otel.JsonContentType, otel.PbContentType)
		return
	}
}

func (f *OTELForwarder) ConfigureRoutes(router *gin.Engine) {
	router.POST("/api/agent/otel/v1/metrics", f.handleMetricsPost)
	pprof.Register(router, "/debug/plugin_logging/pprof")
}
