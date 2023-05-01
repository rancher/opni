package otel

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/otel"
	httputil "github.com/rancher/opni/pkg/util/http"
	"github.com/samber/lo"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	otlpcommonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	otlpmetricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
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
	privileged        bool
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

func WithPrivileged(privileged bool) OTELInitializerOption {
	return func(o *otelInitializerOptions) {
		o.privileged = privileged
	}
}

type OTELForwarder struct {
	colmetricspb.UnsafeMetricsServiceServer

	logger *zap.SugaredLogger
	ClientProvider
}

func (f *OTELForwarder) IsPrivileged() bool {
	return f.ClientProvider.privileged
}

var _ colmetricspb.MetricsServiceServer = (*OTELForwarder)(nil)

func NewOTELForwarder(ctx context.Context, opts ...OTELInitializerOption) OTELForwarder {
	return OTELForwarder{
		ClientProvider: NewClientProvider(ctx, opts...),
		logger:         logger.NewPluginLogger().Named("metrics-otel-forwarder"),
	}
}

func (f *OTELForwarder) Routes() []httputil.GinHandler {
	return []httputil.GinHandler{
		{
			ContentType: http.MethodPost,
			Route:       "/api/agent/otel/v1/metrics",
			HandlerFunc: f.HandleMetricsPost,
		},
	}
}

func (f *OTELForwarder) Export(
	ctx context.Context,
	req *colmetricspb.ExportMetricsServiceRequest,
) (*colmetricspb.ExportMetricsServiceResponse, error) {
	if !f.HasRemoteTarget() {
		return nil, status.Error(codes.Unavailable, "remote OTLP target not available")
	}
	f.logger.Infof("Received %d metrics", lo.Reduce(req.GetResourceMetrics(), func(acc int, rsc *otlpmetricsv1.ResourceMetrics, _ int) int {
		for _, scope := range rsc.GetScopeMetrics() {
			acc += len(scope.Metrics)
		}
		return acc
	}, 0))
	var tenantId string
	if f.IsPrivileged() {
		tenantId = cluster.StreamAuthorizedID(ctx)
	}

	for _, resourceMetrics := range req.GetResourceMetrics() {
		if resourceMetrics == nil {
			continue
		}
		if tenantId != "" && !clusterIdExists(resourceMetrics.Resource.Attributes) {
			resourceMetrics.Resource.Attributes = append(
				resourceMetrics.Resource.Attributes,
				&otlpcommonv1.KeyValue{
					Key: metricsTenantId,
					Value: &otlpcommonv1.AnyValue{
						Value: &otlpcommonv1.AnyValue_StringValue{
							StringValue: tenantId,
						},
					},
				},
			)
		}
	}

	return f.forwardMetricsToRemote(ctx, req)
}

func (f *OTELForwarder) forwardMetricsToRemote(
	ctx context.Context,
	req *colmetricspb.ExportMetricsServiceRequest,
) (*colmetricspb.ExportMetricsServiceResponse, error) {
	remoteTarget, err := f.GetRemoteTarget()
	if err != nil {
		return nil, err
	}
	resp, err := remoteTarget.Export(ctx, req)
	if err != nil {
		f.logger.Errorf("failed to forward to remote target '%s': %s", func() string {
			if f.remoteAddress == "" {
				return "(implicit stream)"
			}
			return f.remoteAddress
		}, err)
	}
	return resp, err
}

func clusterIdExists(attrs []*otlpcommonv1.KeyValue) bool {
	for _, kv := range attrs {
		if kv.GetKey() == metricsTenantId {
			return true
		}
	}
	return false
}

func (f *OTELForwarder) HandleMetricsPost(c *gin.Context) {
	if !f.HasRemoteTarget() {
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
