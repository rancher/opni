package backend

import (
	"context"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ remoteread.RemoteReadAgentServer = (*RemoteReadForwarder)(nil)

type RemoteReadForwarder struct {
	remoteread.UnsafeRemoteReadAgentServer
}

func (fwd *RemoteReadForwarder) Start(ctx context.Context, request *remoteread.StartReadRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (fwd *RemoteReadForwarder) Stop(ctx context.Context, request *remoteread.StopReadRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
