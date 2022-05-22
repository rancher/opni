package gateway

import (
	"context"
	"net"
	"time"

	"github.com/kralicky/totem"
	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/gateway/stream"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type ConnectionHandler interface {
	HandleAgentConnection(context.Context, agent.ClientSet)
}

type StreamServer struct {
	stream.UnsafeStreamServer
	conf       *v1beta1.GatewayConfigSpec
	logger     *zap.SugaredLogger
	handler    ConnectionHandler
	serverOpts []grpc.ServerOption
}

func NewStreamServer(
	cfg *v1beta1.GatewayConfigSpec,
	lg *zap.SugaredLogger,
	handler ConnectionHandler,
	opts ...grpc.ServerOption,
) *StreamServer {
	return &StreamServer{
		conf:       cfg,
		logger:     lg,
		handler:    handler,
		serverOpts: opts,
	}
}

func (s *StreamServer) Connect(stream stream.Stream_ConnectServer) error {
	ts := totem.NewServer(stream)
	cc, errC := ts.Serve()

	ctx, ca := context.WithCancel(stream.Context())
	defer ca()
	go s.handler.HandleAgentConnection(ctx, agent.NewClientSet(cc))

	err := <-errC
	if err != nil {
		s.logger.With(
			zap.Error(err),
		).Info("agent stream disconnected")
	}
	return err
}

func (s *StreamServer) Serve(listener net.Listener) error {
	server := grpc.NewServer(append(s.serverOpts,
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             15 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    15 * time.Second,
			Timeout: 5 * time.Second,
		}),
	)...)
	return server.Serve(listener)
}
