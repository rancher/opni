package cortex

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/distributor/distributorpb"
	"github.com/golang/snappy"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/remotewrite"
	"github.com/weaveworks/common/user"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type remoteWriteForwarder struct {
	remotewrite.UnsafeRemoteWriteServer
	distClient future.Future[distributorpb.DistributorClient]
	httpClient future.Future[*http.Client]
	config     future.Future[*v1beta1.GatewayConfig]
	logger     *zap.SugaredLogger
}

func (f *remoteWriteForwarder) Push(ctx context.Context, payload *remotewrite.Payload) (_ *emptypb.Empty, pushErr error) {
	defer func() {
		if pushErr != nil {
			f.logger.With(
				"err", pushErr,
				"clusterID", payload.AuthorizedClusterID,
			).Error("error pushing metrics to cortex")
		}
	}()
	writeReq := &cortexpb.WriteRequest{}
	buf, err := snappy.Decode(nil, payload.Contents)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to decode payload: %v", err)
	}
	if err := writeReq.Unmarshal(buf); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to unmarshal decompressed payload: %v", err)
	}
	ctx, err = user.InjectIntoGRPCRequest(user.InjectOrgID(ctx, payload.AuthorizedClusterID))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid org id: %v", err)
	}
	_, err = f.distClient.Get().Push(ctx, writeReq)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (f *remoteWriteForwarder) SyncRules(ctx context.Context, payload *remotewrite.Payload) (_ *emptypb.Empty, syncErr error) {
	defer func() {
		if syncErr != nil {
			f.logger.With(
				"err", syncErr,
				"clusterID", payload.AuthorizedClusterID,
			).Error("error syncing rules to cortex")
		}
	}()
	url := fmt.Sprintf("https://%s/api/v1/rules/%s",
		f.config.Get().Spec.Cortex.Ruler.HTTPAddress, payload.AuthorizedClusterID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		bytes.NewReader(payload.Contents))
	if err != nil {
		return nil, err
	}
	if err := user.InjectOrgIDIntoHTTPRequest(user.InjectOrgID(ctx, payload.AuthorizedClusterID), req); err != nil {
		return nil, err
	}
	for k, v := range payload.Headers {
		req.Header.Add(k, v)
	}
	resp, err := f.httpClient.Get().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, status.Errorf(codes.Internal, "failed to sync rules: %v", resp.Status)
	}
	return &emptypb.Empty{}, nil
}
