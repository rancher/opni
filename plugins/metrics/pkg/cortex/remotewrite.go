package cortex

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
	"github.com/weaveworks/common/user"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type RemoteWriteForwarder struct {
	remotewrite.UnsafeRemoteWriteServer
	RemoteWriteForwarderConfig

	util.Initializer
}

var _ remotewrite.RemoteWriteServer = (*RemoteWriteForwarder)(nil)

type RemoteWriteForwarderConfig struct {
	CortexClientSet ClientSet                  `validate:"required"`
	Config          *v1beta1.GatewayConfigSpec `validate:"required"`
	Logger          *zap.SugaredLogger         `validate:"required"`
}

func (f *RemoteWriteForwarder) Initialize(conf RemoteWriteForwarderConfig) {
	f.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		f.RemoteWriteForwarderConfig = conf
	})
}

func (f *RemoteWriteForwarder) Push(ctx context.Context, payload *remotewrite.Payload) (_ *emptypb.Empty, pushErr error) {
	if !f.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	//clusterId, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
	//if !ok {
	//	return nil, status.Error(codes.Unauthenticated, "no cluster ID found in context")
	//}

	clusterId := "cluster-0"

	defer func() {
		code := status.Code(pushErr)
		mRemoteWriteRequests.WithLabelValues(clusterId, fmt.Sprint(code), code.String()).Inc()
	}()

	payloadSize := float64(len(payload.Contents))
	mIngestBytesTotal.Add(payloadSize)
	mIngestBytesByID.WithLabelValues(clusterId).Add(payloadSize)

	ctx, span := otel.Tracer("plugin_metrics").Start(ctx, "remoteWriteForwarder.Push",
		trace.WithAttributes(attribute.String("clusterId", clusterId)))
	defer span.End()

	defer func() {
		if pushErr != nil {
			lg := f.Logger.With(
				"err", pushErr,
				"clusterId", clusterId,
			)
			if s, ok := status.FromError(pushErr); ok {
				lg = lg.With("code", int32(s.Code()))
			}
			lg.Error("error pushing metrics to cortex")
		}
	}()
	url := fmt.Sprintf("https://%s/api/v1/push", f.Config.Cortex.Distributor.HTTPAddress)
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
		return nil, status.Errorf(codes.Internal, "error pushing metrics to cortex: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, status.New(codes.Code(resp.StatusCode), string(msg)).Err() // match cortex logic
	}
	return &emptypb.Empty{}, nil
}

func (f *RemoteWriteForwarder) SyncRules(ctx context.Context, payload *remotewrite.Payload) (_ *emptypb.Empty, syncErr error) {
	if !f.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	clusterId, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no cluster ID found in context")
	}

	ctx, span := otel.Tracer("plugin_metrics").Start(ctx, "remoteWriteForwarder.Push",
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
