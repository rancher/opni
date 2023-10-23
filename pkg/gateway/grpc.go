package gateway

import (
	"context"
	"net"
	"runtime"
	"sync"
	"time"

	"log/slog"

	"github.com/samber/lo"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"

	"github.com/rancher/opni/pkg/agent"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util"
)

type ConnectionHandler interface {
	HandleAgentConnection(context.Context, agent.ClientSet)
}

func MultiConnectionHandler(handlers ...ConnectionHandler) ConnectionHandler {
	return &multiConnectionHandler{
		handlers: handlers,
	}
}

type multiConnectionHandler struct {
	handlers []ConnectionHandler
}

func (m *multiConnectionHandler) HandleAgentConnection(ctx context.Context, clientSet agent.ClientSet) {
	for _, handler := range m.handlers {
		go handler.HandleAgentConnection(ctx, clientSet)
	}
}

type ConnectionHandlerFunc func(context.Context, agent.ClientSet)

func (f ConnectionHandlerFunc) HandleAgentConnection(ctx context.Context, clientSet agent.ClientSet) {
	f(ctx, clientSet)
}

type GatewayGRPCServer struct {
	streamv1.UnsafeStreamServer
	conf       *v1beta1.GatewayConfigSpec
	logger     *slog.Logger
	serverOpts []grpc.ServerOption

	servicesMu sync.Mutex
	services   []util.ServicePack[any]
}

func NewGRPCServer(
	cfg *v1beta1.GatewayConfigSpec,
	lg *slog.Logger,
	opts ...grpc.ServerOption,
) *GatewayGRPCServer {
	return &GatewayGRPCServer{
		conf:       cfg,
		logger:     lg.WithGroup("grpc"),
		serverOpts: opts,
	}
}

func (s *GatewayGRPCServer) ListenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp4", s.conf.GRPCListenAddress)
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
		grpc.ChainStreamInterceptor(otelgrpc.StreamServerInterceptor()),
		grpc.ChainUnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
		grpc.ReadBufferSize(0),
		grpc.WriteBufferSize(0),
		grpc.NumStreamWorkers(uint32(runtime.NumCPU())),
		grpc.MaxHeaderListSize(1024*8),
	)...)
	healthv1.RegisterHealthServer(server, health.NewServer())
	s.servicesMu.Lock()
	for _, services := range s.services {
		server.RegisterService(services.Unpack())
	}
	s.servicesMu.Unlock()

	s.logger.With(
		"address", listener.Addr().String(),
	).Info("gateway gRPC server starting")

	errC := lo.Async(func() error {
		return server.Serve(listener)
	})
	select {
	case <-ctx.Done():
		server.Stop()
		return (<-errC)
	case err := <-errC:
		return err
	}
}

func (s *GatewayGRPCServer) RegisterService(desc *grpc.ServiceDesc, impl any) {
	s.servicesMu.Lock()
	defer s.servicesMu.Unlock()
	s.services = append(s.services, util.PackService(desc, impl))
}
