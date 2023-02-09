package backend

import (
	"context"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ remoteread.RemoteReadGatewayServer = (*RemoteReadForwarder)(nil)

type RemoteReadForwarderConfig struct {
	client remoteread.RemoteReadGatewayClient
}

type RemoteReadForwarder struct {
	// todo: add missing functions (add, edit, rm, etc) this is fine for testing but bad for production
	remoteread.UnimplementedRemoteReadGatewayServer
	RemoteReadForwarderConfig
	util.Initializer
}

func (fwd *RemoteReadForwarder) Initialize(conf RemoteReadForwarderConfig) {
	fwd.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		fwd.RemoteReadForwarderConfig = conf
	})
}

func (fwd *RemoteReadForwarder) Start(ctx context.Context, request *remoteread.StartReadRequest) (*emptypb.Empty, error) {
	panic("not yet implemented")
	//return &emptypb.Empty{}, nil
}

func (fwd *RemoteReadForwarder) Stop(ctx context.Context, request *remoteread.StopReadRequest) (*emptypb.Empty, error) {
	panic("not yet implemented")
	//return &emptypb.Empty{}, nil
}

func (fwd *RemoteReadForwarder) UpdateTargetStatus(ctx context.Context, request *remoteread.TargetStatusUpdateRequest) (*emptypb.Empty, error) {
	return fwd.client.UpdateTargetStatus(ctx, request)
}
