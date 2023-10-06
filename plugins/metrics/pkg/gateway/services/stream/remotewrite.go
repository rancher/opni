package stream

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/auth/session"
	"github.com/rancher/opni/pkg/metrics"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
	"github.com/weaveworks/common/user"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	cIngestBytesByID     metric.Int64Counter
	cRemoteWriteRequests metric.Int64Counter
)

type RemoteWriteServer struct {
	sc types.StreamServiceContext

	interceptors    map[string]cortex.RequestInterceptor
	cortexClientSet cortex.ClientSet
}

var _ remotewrite.RemoteWriteServer = (*RemoteWriteServer)(nil)

func (f *RemoteWriteServer) Activate(ctx types.StreamServiceContext) error {
	clientSet, err := ctx.Memoize(cortex.ClientSetKey, func() (any, error) {
		tlsConfig, err := cortex.LoadTLSConfig(ctx.GatewayConfig())
		if err != nil {
			return nil, err
		}
		return cortex.NewClientSet(ctx, &ctx.GatewayConfig().Spec.Cortex, tlsConfig)
	})
	if err != nil {
		return err
	}
	f.cortexClientSet = clientSet.(cortex.ClientSet)
	f.interceptors = map[string]cortex.RequestInterceptor{
		"local": cortex.NewFederatingInterceptor(cortex.InterceptorConfig{
			IdLabelName: metrics.LabelImpersonateAs,
		}),
	}
	f.sc = ctx
	return nil
}

var passthrough = &cortex.PassthroughInterceptor{}

func (f *RemoteWriteServer) Push(ctx context.Context, writeReq *cortexpb.WriteRequest) (_ *cortexpb.WriteResponse, pushErr error) {
	clusterId := cluster.StreamAuthorizedID(ctx)

	var interceptor cortex.RequestInterceptor = passthrough
	attributes := session.StreamAuthorizedAttributes(ctx)
	for _, attr := range attributes {
		if i, ok := f.interceptors[attr.Name()]; ok {
			interceptor = i
			break
		}
	}

	defer func() {
		code := status.Code(pushErr)
		cRemoteWriteRequests.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("cluster_id", clusterId),
				attribute.Int("code", int(code)),
				attribute.String("code_text", code.String()),
			),
		)
	}()

	payloadSize := int64(writeReq.Size())
	cIngestBytesByID.Add(ctx, payloadSize,
		metric.WithAttributes(
			attribute.String("cluster_id", clusterId),
		),
	)

	ctx, span := otel.Tracer("plugin_metrics").Start(ctx, "remoteWriteForwarder.Push",
		trace.WithAttributes(attribute.String("clusterId", clusterId)))
	defer span.End()

	defer func() {
		if pushErr != nil {
			if s, ok := status.FromError(pushErr); ok {
				code := s.Code()
				if code == 400 {
					// As a special case, status code 400 may indicate a success.
					// Cortex handles a variety of cases where prometheus would normally
					// return an error, such as duplicate or out of order samples. Cortex
					// will return code 400 to prometheus, which prometheus will treat as
					// a non-retriable error. In this case, the remote write status condition
					// will be cleared as if the request succeeded.
					if code == http.StatusBadRequest {
						message := s.Message()
						if strings.Contains(message, "out of bounds") ||
							strings.Contains(message, "out of order sample") ||
							strings.Contains(message, "duplicate sample for timestamp") ||
							strings.Contains(message, "exemplars not ingested because series not already present") {
							{
								// clear the soft error
								pushErr = nil
							}
						}
					}
				}
			}
		}
	}()

	var err error
	ctx, err = user.InjectIntoGRPCRequest(user.InjectOrgID(ctx, clusterId))
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	wr, err := interceptor.Intercept(ctx, writeReq, f.cortexClientSet.Distributor().Push)

	cortexpb.ReuseSlice(writeReq.Timeseries)

	return wr, err
}

func (f *RemoteWriteServer) SyncRules(ctx context.Context, payload *remotewrite.Payload) (_ *emptypb.Empty, syncErr error) {
	clusterId := cluster.StreamAuthorizedID(ctx)

	ctx, span := otel.Tracer("plugin_metrics").Start(ctx, "remoteWriteForwarder.SyncRules",
		trace.WithAttributes(attribute.String("clusterId", clusterId)))
	defer span.End()

	defer func() {
		if syncErr != nil {
			f.sc.Logger().With(
				"err", syncErr,
				"clusterId", clusterId,
			).Error("error syncing rules to cortex")
		}
	}()
	url := fmt.Sprintf(
		"https://%s/api/v1/rules/%s",
		f.sc.GatewayConfig().Spec.Cortex.Ruler.HTTPAddress,
		"synced", // set the namespace to synced to differentiate from user rules
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		bytes.NewReader(payload.Contents))
	if err != nil {
		return nil, err
	}
	if err := user.InjectOrgIDIntoHTTPRequest(user.InjectOrgID(ctx, clusterId), req); err != nil {
		return nil, err
	}
	for k, v := range payload.Headers {
		req.Header.Add(k, v)
	}
	resp, err := f.cortexClientSet.HTTP().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, status.Errorf(codes.Internal, "failed to sync rules: %v", resp.Status)
	}
	return &emptypb.Empty{}, nil
}
