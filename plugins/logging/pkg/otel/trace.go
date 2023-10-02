package otel

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/supportagent"
	"github.com/rancher/opni/plugins/logging/pkg/util"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	otlpcommonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type TraceForwarder struct {
	coltracepb.UnsafeTraceServiceServer
	forwarderOptions

	Client   *util.AsyncClient[coltracepb.TraceServiceClient]
	clientMu sync.RWMutex
}

func NewTraceForwarder(opts ...ForwarderOption) *TraceForwarder {
	options := forwarderOptions{
		collectorAddressOverride: defaultAddress,
		lg:                       logger.NewPluginLogger().Named("default-otel"),
	}
	options.apply(opts...)
	return &TraceForwarder{
		forwarderOptions: options,
		Client:           util.NewAsyncClient[coltracepb.TraceServiceClient](),
	}
}

func (f *TraceForwarder) SetClient(cc grpc.ClientConnInterface) {
	f.clientMu.Lock()
	defer f.clientMu.Unlock()

	client := coltracepb.NewTraceServiceClient(cc)
	f.Client.SetClient(client)
}

func (f *TraceForwarder) InitializeTraceForwarder() coltracepb.TraceServiceClient {
	if f.cc == nil {
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
				f.lg.Warn("plugin context cancelled before gRPC client created")
				return nil
			case <-b.Next():
				conn, err := grpc.Dial(
					f.collectorAddressOverride,
					f.dialOptions...,
				)
				if err != nil {
					f.lg.Errorf("failed dial grpc: %v", err)
					continue
				}
				return coltracepb.NewTraceServiceClient(conn)
			}
		}
	}
	return coltracepb.NewTraceServiceClient(f.cc)
}

func (f *TraceForwarder) Export(
	ctx context.Context,
	request *coltracepb.ExportTraceServiceRequest,
) (*coltracepb.ExportTraceServiceResponse, error) {
	if !f.Client.IsSet() {
		f.lg.Error("collector is unavailable")
		return nil, status.Errorf(codes.Unavailable, "collector is unavailable")
	}

	if f.privileged {
		clusterID := cluster.StreamAuthorizedID(ctx)
		addValueToSpanAttributes(request, clusterIDKey, clusterID)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return f.forwardTrace(ctx, request)
	}

	values := md.Get(supportagent.AttributeValuesKey)

	if len(values) < 2 {
		return f.forwardTrace(ctx, request)
	}

	if len(values)%2 != 0 {
		f.lg.Warnf("invalid number of attribute values: %d", len(values))
		return f.forwardTrace(ctx, request)
	}

	for i := 0; i < len(values); i += 2 {
		key := values[i]
		value := values[i+1]
		addValueToSpanAttributes(request, key, value)
	}

	return f.forwardTrace(ctx, request)
}

func addValueToSpanAttributes(request *coltracepb.ExportTraceServiceRequest, key, value string) {
	resourceSpans := request.GetResourceSpans()
	for _, resourceSpan := range resourceSpans {
		scopeSpans := resourceSpan.GetScopeSpans()
		for _, scopeSpan := range scopeSpans {
			spans := scopeSpan.GetSpans()
			for _, span := range spans {
				if span != nil && !keyExists(span.GetAttributes(), key) {
					span.Attributes = append(span.Attributes, &otlpcommonv1.KeyValue{
						Key: key,
						Value: &otlpcommonv1.AnyValue{
							Value: &otlpcommonv1.AnyValue_StringValue{
								StringValue: value,
							},
						},
					})
				}
			}
		}
		resourceSpan.ScopeSpans = scopeSpans
	}
	request.ResourceSpans = resourceSpans
}

func (f *TraceForwarder) forwardTrace(
	ctx context.Context,
	request *coltracepb.ExportTraceServiceRequest,
) (*coltracepb.ExportTraceServiceResponse, error) {
	resp, err := f.Client.Client.Export(ctx, request)
	if err != nil {
		f.lg.Error("failed to forward traces: %v", err)
		return nil, err
	}
	return resp, nil
}

func (f *TraceForwarder) handleTracePost(c *gin.Context) {
	f.clientMu.RLock()
	defer f.clientMu.RUnlock()
	if !f.Client.IsSet() {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	switch c.ContentType() {
	case pbContentType:
		f.renderProto(c)
	case jsonContentType:
		f.renderProtoJSON(c)
	default:
		c.String(http.StatusUnsupportedMediaType, "unsupported media type, supported: [%s,%s]", jsonContentType, pbContentType)
		return
	}
}
