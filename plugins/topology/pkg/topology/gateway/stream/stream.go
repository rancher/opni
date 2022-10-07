package stream

/*
Implements logic to push topology data to the remote endpoint (gateway).

The remote write server lives on the gateway and agents acquire a client
to stream to.
*/

import (
	"context"

	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/topology/pkg/apis/remote"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
)

type TopologyRemoteWriteConfig struct {
	Logger *zap.SugaredLogger
}

type TopologyRemoteWriter struct {
	remote.UnsafeRemoteTopologyServer
	TopologyRemoteWriteConfig

	util.Initializer
}

var _ remote.RemoteTopologyServer = (*TopologyRemoteWriter)(nil)

func (t *TopologyRemoteWriter) Initialize(conf TopologyRemoteWriteConfig) {
	t.InitOnce(func() {
		// TODO : initialization code goes here
	})
}

func (t *TopologyRemoteWriter) Push(ctx context.Context, payload *remote.Payload) (*emptypb.Empty, error) {
	if !t.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}

	// TODO : implement me
	return nil, nil
}

func (t *TopologyRemoteWriter) SyncTopology(ctx context.Context, payload *remote.Payload) (*emptypb.Empty, error) {
	if !t.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}

	// TODO : implement me

	return nil, nil
}
