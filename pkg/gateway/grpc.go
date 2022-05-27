package gateway

import (
	"context"
	"net"
	"time"

	"github.com/rancher/opni/pkg/agent"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
)

type ConnectionHandler interface {
	HandleAgentConnection(context.Context, agent.ClientSet)
}

type service struct {
	desc *grpc.ServiceDesc
	impl any
}

type GatewayGRPCServer struct {
	streamv1.UnsafeStreamServer
	conf       *v1beta1.GatewayConfigSpec
	logger     *zap.SugaredLogger
	serverOpts []grpc.ServerOption

	services []service
}

func NewGRPCServer(
	cfg *v1beta1.GatewayConfigSpec,
	lg *zap.SugaredLogger,
	opts ...grpc.ServerOption,
) *GatewayGRPCServer {
	return &GatewayGRPCServer{
		conf:       cfg,
		logger:     lg.Named("grpc"),
		serverOpts: opts,
	}
}

func (s *GatewayGRPCServer) ListenAndServe(ctx context.Context) error {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp4", s.conf.GRPCListenAddress)
	if err != nil {
		return err
	}
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
	healthv1.RegisterHealthServer(server, health.NewServer())
	for _, services := range s.services {
		server.RegisterService(services.desc, services.impl)
	}

	s.logger.With(
		"address", listener.Addr().String(),
	).Info("gateway gRPC server starting")

	return server.Serve(listener)
}

func (s *GatewayGRPCServer) RegisterService(desc *grpc.ServiceDesc, impl any) {
	s.services = append(s.services, service{
		desc: desc,
		impl: impl,
	})
}
