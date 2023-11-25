package gateway

import (
	"context"
	"crypto/tls"
	"net"
	"runtime"
	"sync"
	"time"

	"log/slog"

	"github.com/samber/lo"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/rancher/opni/pkg/agent"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/config/reactive"
	configv1 "github.com/rancher/opni/pkg/config/v1"
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
	mgr        *configv1.GatewayConfigManager
	logger     *slog.Logger
	serverOpts []grpc.ServerOption

	servicesMu sync.Mutex
	services   []util.ServicePack[any]
}

func NewGRPCServer(
	mgr *configv1.GatewayConfigManager,
	lg *slog.Logger,
	opts ...grpc.ServerOption,
) *GatewayGRPCServer {
	return &GatewayGRPCServer{
		mgr:        mgr,
		logger:     lg.WithGroup("grpc"),
		serverOpts: opts,
	}
}

func (s *GatewayGRPCServer) ListenAndServe(ctx context.Context) (serveError error) {
	var cancel context.CancelFunc
	var done chan struct{}
	doServe := func(addr string, certs *configv1.CertsSpec) error {
		if cancel != nil {
			cancel()
			select {
			case <-ctx.Done():
				return nil
			case <-done:
				// server stopped, continue
			}
		}
		done = make(chan struct{})
		var serveCtx context.Context
		serveCtx, cancel = context.WithCancel(ctx)
		listener, err := net.Listen("tcp4", addr)
		if err != nil {
			return err
		}
		tlsConfig, err := certs.AsTlsConfig(tls.NoClientCert)
		if err != nil {
			return err
		}
		go func() {
			s.serve(serveCtx, listener, tlsConfig)
			close(done)
		}()
		return nil
	}

	reactive.Bind(ctx,
		func(v []protoreflect.Value) {
			serveError = doServe(v[0].String(), v[1].Message().Interface().(*configv1.CertsSpec))
		},
		s.mgr.Reactive(configv1.ProtoPath().Server().GrpcListenAddress()),
		s.mgr.Reactive(protopath.Path(configv1.ProtoPath().Certs())),
	)
	<-ctx.Done()
	return
}

func (s *GatewayGRPCServer) serve(ctx context.Context, listener net.Listener, tlsConfig *tls.Config) error {
	server := grpc.NewServer(append(s.serverOpts,
		grpc.Creds(credentials.NewTLS(tlsConfig)),
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
