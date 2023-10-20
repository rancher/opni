package cortex

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/auth/session"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/metrics"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
	"github.com/weaveworks/common/user"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"log/slog"
)

type RemoteWriteForwarder struct {
	remotewrite.UnsafeRemoteWriteServer
	RemoteWriteForwarderConfig
	interceptors map[string]RequestInterceptor

	util.Initializer
}

var _ remotewrite.RemoteWriteServer = (*RemoteWriteForwarder)(nil)

type RemoteWriteForwarderConfig struct {
	CortexClientSet ClientSet                  `validate:"required"`
	Config          *v1beta1.GatewayConfigSpec `validate:"required"`
	Logger          *slog.Logger               `validate:"required"`
}

func (f *RemoteWriteForwarder) Initialize(conf RemoteWriteForwarderConfig) {
	f.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		f.RemoteWriteForwarderConfig = conf

		f.interceptors = map[string]RequestInterceptor{
			"local": NewFederatingInterceptor(InterceptorConfig{
				IdLabelName: metrics.LabelImpersonateAs,
			}),
		}
	})
}

var passthrough = &passthroughInterceptor{}

func (f *RemoteWriteForwarder) Push(ctx context.Context, writeReq *cortexpb.WriteRequest) (_ *cortexpb.WriteResponse, pushErr error) {
	if !f.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}

	clusterId := cluster.StreamAuthorizedID(ctx)

	var interceptor RequestInterceptor = passthrough
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

	wr, err := interceptor.Intercept(ctx, writeReq, f.CortexClientSet.Distributor().Push)

	cortexpb.ReuseSlice(writeReq.Timeseries)

	return wr, err
}

func (f *RemoteWriteForwarder) SyncRules(ctx context.Context, payload *remotewrite.Payload) (_ *emptypb.Empty, syncErr error) {
	if !f.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	clusterId := cluster.StreamAuthorizedID(ctx)

	ctx, span := otel.Tracer("plugin_metrics").Start(ctx, "remoteWriteForwarder.SyncRules",
		trace.WithAttributes(attribute.String("clusterId", clusterId)))
	defer span.End()

	defer func() {
		if syncErr != nil {
			f.Logger.With(
				"err", syncErr,
				"clusterId", clusterId,
			).Error("error syncing rules to cortex")
		}
	}()
	url := fmt.Sprintf(
		"https://%s/api/v1/rules/%s",
		f.Config.Cortex.Ruler.HTTPAddress,
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
	resp, err := f.CortexClientSet.HTTP().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, status.Errorf(codes.Internal, "failed to sync rules: %v", resp.Status)
	}
	return &emptypb.Empty{}, nil
}
