package syncer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"time"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/samber/lo"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func Main(
	runCtx context.Context,
	cfg *alertingv1.SyncerConfig,
	tlsConfig *tls.Config,
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

	server := grpc.NewServer(
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             15 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    15 * time.Second,
			Timeout: 5 * time.Second,
		}),
	)

	mgmtClient, err := clients.NewManagementClient(runCtx, clients.WithAddress(cfg.GatewayJoinAddress))
	if err != nil {
		panic(err)
	}

	alertingv1.RegisterSyncerServer(
		server,
		NewAlertingSyncerV1( // run with defaults
			runCtx,
			cfg,
			mgmtClient,
			tlsConfig,
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
