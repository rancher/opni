package services

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
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
	"github.com/weaveworks/common/user"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/tools/pkg/memoize"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type RemoteWriteServer struct {
	Context types.StreamServiceContext `option:"context"`

	interceptors    map[string]cortex.RequestInterceptor
	cortexClientSet *memoize.Promise
}

var _ remotewrite.RemoteWriteServer = (*RemoteWriteServer)(nil)

func (s *RemoteWriteServer) Activate() error {
	s.cortexClientSet = s.Context.Memoize(cortex.NewClientSet(s.Context.GatewayConfig()))
	s.interceptors = map[string]cortex.RequestInterceptor{
		"local": cortex.NewFederatingInterceptor(cortex.InterceptorConfig{
			IdLabelName: metrics.LabelImpersonateAs,
		}),
	}
	return nil
}

// StreamServices implements types.StreamService
func (s *RemoteWriteServer) StreamServices() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[remotewrite.RemoteWriteServer](&remotewrite.RemoteWrite_ServiceDesc, s),
	}
}

var passthrough = &cortex.PassthroughInterceptor{}

func (s *RemoteWriteServer) Push(ctx context.Context, writeReq *cortexpb.WriteRequest) (_ *cortexpb.WriteResponse, pushErr error) {
	clusterId := cluster.StreamAuthorizedID(ctx)

	var interceptor cortex.RequestInterceptor = passthrough
	attributes := session.StreamAuthorizedAttributes(ctx)
	for _, attr := range attributes {
		if i, ok := s.interceptors[attr.Name()]; ok {
			interceptor = i
			break
		}
	}

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

	cs, err := cortex.AcquireClientSet(ctx, s.cortexClientSet)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	wr, err := interceptor.Intercept(ctx, writeReq, cs.Distributor().Push)

	cortexpb.ReuseSlice(writeReq.Timeseries)

	return wr, err
}

func (s *RemoteWriteServer) SyncRules(ctx context.Context, payload *remotewrite.Payload) (_ *emptypb.Empty, syncErr error) {
	clusterId := cluster.StreamAuthorizedID(ctx)

	ctx, span := otel.Tracer("plugin_metrics").Start(ctx, "remoteWriteForwarder.SyncRules",
		trace.WithAttributes(attribute.String("clusterId", clusterId)))
	defer span.End()

	defer func() {
		if syncErr != nil {
			s.Context.Logger().With(
				"err", syncErr,
				"clusterId", clusterId,
			).Error("error syncing rules to cortex")
		}
	}()
	url := fmt.Sprintf(
		"https://%s/api/v1/rules/%s",
		s.Context.GatewayConfig().Spec.Cortex.Ruler.HTTPAddress,
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
	cs, err := cortex.AcquireClientSet(ctx, s.cortexClientSet)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	resp, err := cs.HTTP().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, status.Errorf(codes.Internal, "failed to sync rules: %v", resp.Status)
	}
	return &emptypb.Empty{}, nil
}

func init() {
	types.Services.Register("Remote Write Stream Service", func(_ context.Context, opts ...driverutil.Option) (types.Service, error) {
		svc := &RemoteWriteServer{}
		driverutil.ApplyOptions(svc, opts...)
		return svc, nil
	})
}
