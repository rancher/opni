package otel

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/supportagent"
	"github.com/rancher/opni/plugins/logging/pkg/util"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	otlpcommonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	defaultAddress = "http://localhost:8080"
	clusterIDKey   = "cluster_id"
	nodeNameKey    = "node_name"
	caseNumberKey  = "case_number"
)

type LogsForwarder struct {
	collogspb.UnsafeLogsServiceServer
	forwarderOptions

	Client   *util.AsyncClient[collogspb.LogsServiceClient]
	clientMu sync.RWMutex
}

func NewLogsForwarder(ctx context.Context, opts ...ForwarderOption) *LogsForwarder {
	lg := logger.PluginLoggerFromContext(ctx)
	if lg == nil {
		lg = logger.NewPluginLogger(ctx).WithGroup("default-otel")
	}

	options := forwarderOptions{
		collectorAddressOverride: defaultAddress,
		ctx:                      logger.WithPluginLogger(ctx, lg),
	}
	options.apply(opts...)
	return &LogsForwarder{
		forwarderOptions: options,
		Client:           util.NewAsyncClient[collogspb.LogsServiceClient](),
	}
}

func (f *LogsForwarder) SetClient(cc grpc.ClientConnInterface) {
	f.clientMu.Lock()
	defer f.clientMu.Unlock()

	client := collogspb.NewLogsServiceClient(cc)
	f.Client.SetClient(client)
}

func (f *LogsForwarder) initializeLogsForwarder() collogspb.LogsServiceClient {
	lg := logger.PluginLoggerFromContext(f.ctx)
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
				lg.Warn("plugin context cancelled before gRPC client created")
				return nil
			case <-b.Next():
				conn, err := grpc.Dial(
					f.collectorAddressOverride,
					f.dialOptions...,
				)
				if err != nil {
					lg.Error(fmt.Sprintf("failed dial grpc: %v", err))
					continue
				}
				return collogspb.NewLogsServiceClient(conn)
			}
		}
	}
	return collogspb.NewLogsServiceClient(f.cc)
}

func (f *LogsForwarder) Export(
	ctx context.Context,
	request *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	lg := logger.PluginLoggerFromContext(f.ctx)
	if !f.Client.IsSet() {
		lg.Error("collector is unavailable")
		return nil, status.Errorf(codes.Unavailable, "collector is unavailable")
	}
	clusterID := cluster.StreamAuthorizedID(ctx)
	addValueToResource(request, clusterIDKey, clusterID)

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return f.forwardLogs(ctx, request)
	}

	values := md.Get(supportagent.AttributeValuesKey)

	if len(values) < 2 {
		return f.forwardLogs(ctx, request)
	}

	if len(values)%2 != 0 {
		lg.Warn(fmt.Sprintf("invalid number of attribute values: %d", len(values)))
		return f.forwardLogs(ctx, request)
	}

	for i := 0; i < len(values); i += 2 {
		key := values[i]
		value := values[i+1]
		addValueToResource(request, key, value)
	}

	return f.forwardLogs(ctx, request)
}

func addValueToResource(request *collogspb.ExportLogsServiceRequest, key, value string) {
	logs := request.GetResourceLogs()
	for _, log := range logs {
		resource := log.GetResource()
		if resource != nil && !keyExists(resource.GetAttributes(), key) {
			resource.Attributes = append(resource.Attributes, &otlpcommonv1.KeyValue{
				Key: key,
				Value: &otlpcommonv1.AnyValue{
					Value: &otlpcommonv1.AnyValue_StringValue{
						StringValue: value,
					},
				},
			})
		}
	}
	request.ResourceLogs = logs
}

func keyExists(attr []*otlpcommonv1.KeyValue, key string) bool {
	for _, kv := range attr {
		if kv.GetKey() == key {
			return true
		}
	}
	return false
}

func (f *LogsForwarder) forwardLogs(
	ctx context.Context,
	request *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	lg := logger.PluginLoggerFromContext(f.ctx)
	resp, err := f.Client.Client.Export(ctx, request)
	if err != nil {
		lg.Error("failed to forward logs: %v", logger.Err(err))
		return nil, err
	}
	return resp, nil
}

func (f *LogsForwarder) handleLogsPost(c *gin.Context) {
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
