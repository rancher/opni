package syncer

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"time"

	"github.com/rancher/opni/pkg/alerting/extensions"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/samber/lo"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	"net/http"
	_ "net/http/pprof"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func Main(
	runCtx context.Context,
	cfg *alertingv1.SyncerConfig,
) error {
	if cfg.ProfileBlockRate > 0 && cfg.PprofPort > 0 {
		runtime.SetBlockProfileRate(int(cfg.ProfileBlockRate))
		go func() {
			http.ListenAndServe(fmt.Sprintf(":%d", cfg.PprofPort), nil)
		}()
	}

	listener, err := net.Listen("tcp4", cfg.ListenAddress)
	if err != nil {
		panic(err)
	}

	//TODO: are these good default GRPC settings?
	server := grpc.NewServer(
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
		grpc.MaxRecvMsgSize(32*1024*1024), // 32MB
		grpc.ReadBufferSize(0),
		grpc.NumStreamWorkers(uint32(runtime.NumCPU())),
		grpc.InitialConnWindowSize(64*1024*1024), // 64MB
		grpc.InitialWindowSize(64*1024*1024),     // 64MB
	)

	mgmtClient, err := clients.NewManagementClient(runCtx, clients.WithAddress(cfg.GatewayJoinAddress))

	alertingv1.RegisterSyncerServer(
		server,
		extensions.NewAlertingSyncerV1( // run with defaults
			runCtx,
			cfg,
			mgmtClient,
		),
	)

	errC := lo.Async(func() error {
		return server.Serve(listener)
	})

	select {
	case <-runCtx.Done():
		server.GracefulStop()
		return runCtx.Err()
	case err := <-errC:
		return err
	}
}
