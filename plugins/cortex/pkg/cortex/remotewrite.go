package cortex

import (
	"bytes"
	"context"
	"net/http"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/distributor/distributorpb"
	"github.com/golang/snappy"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/remotewrite"
	"github.com/weaveworks/common/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type remoteWriteForwarder struct {
	remotewrite.UnsafeRemoteWriteServer
	distClient *util.Future[distributorpb.DistributorClient]
	httpClient *util.Future[*http.Client]
}

func (f *remoteWriteForwarder) Push(ctx context.Context, payload *remotewrite.Payload) (*emptypb.Empty, error) {
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

func (f *remoteWriteForwarder) SyncRules(ctx context.Context, payload *remotewrite.Payload) (*emptypb.Empty, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/v1/rules/"+payload.AuthorizedClusterID,
		bytes.NewReader(payload.Contents),
	)
	if err != nil {
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
	if resp.StatusCode != http.StatusOK {
		return nil, status.Errorf(codes.Internal, "failed to sync rules: %v", resp.Status)
	}
	return &emptypb.Empty{}, nil
}
