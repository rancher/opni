/*
Code in this package is meant for testing only. It is not used for production process sychronization.

The command built using this package uses non-standard exit codes.
*/
package lock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/etcd"
	"github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/validation"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildLockerServerCommand() *cobra.Command {
	var configFile string
	var listenAddr string
	cmd := &cobra.Command{
		Use:          "server",
		Short:        "distributed lock broker server",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLockServer(cmd.Context(), configFile, listenAddr)
		},
	}
	cmd.Flags().StringVarP(&configFile, "config", "f", "/tmp/lock.json", "lock backend config file")
	cmd.Flags().StringVarP(&listenAddr, "listen-addr", "a", "127.0.0.1:5002", "lock server listen address")
	return cmd
}

func BuildClientCommand() *cobra.Command {
	serverCmd := testgrpc.BuildTestLockerCmd()
	serverCmd.Use = "client"
	serverCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		addr := os.Getenv("LOCK_SERVER_ADDRESS")
		if addr == "" {
			addr = "127.0.0.1:5002"
		}
		cmd.Println("connecting to", addr)
		cc, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return err
		}
		client := testgrpc.NewTestLockerClient(cc)
		cmd.SetContext(testgrpc.ContextWithTestLockerClient(cmd.Context(), client))
		return nil
	}

	return serverCmd
}

func BuildRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dlock",
		Short: "distributed locking commands",
	}
	cmd.SilenceUsage = true
	cmd.AddCommand(BuildLockerServerCommand())
	cmd.AddCommand(BuildClientCommand())
	return cmd
}

func runLockServer(
	ctx context.Context,
	configFile,
	listenAddress string,
) error {
	lg := logger.NewPluginLogger().WithGroup("lock-server")
	lockServer, err := NewLockServer(lg, configFile)
	if err != nil {
		lg.Error(err.Error())
		return err
	}

	listener, err := net.Listen("tcp4", listenAddress)
	if err != nil {
		lg.Error(err.Error())
		return err
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

	testgrpc.RegisterTestLockerServer(
		server,
		lockServer,
	)
	errC := lo.Async(func() error {
		return server.Serve(listener)
	})
	lg.Info(fmt.Sprintf("serving lock server on %s", listenAddress))
	select {
	case <-ctx.Done():
		lg.Info("context done")
		return nil
	case err := <-errC:
		lg.Error(err.Error())
		return err
	}

}

type LockServer struct {
	testgrpc.UnsafeTestLockerServer

	lg *slog.Logger
	lm storage.LockManager

	hackMu sync.Mutex
	hack   map[string]storage.Lock
}

func NewLockServer(
	lg *slog.Logger,
	configFile string,
) (*LockServer, error) {
	lm, err := setup(lg, configFile)
	if err != nil {
		return nil, err
	}
	return &LockServer{
		lm:   lm,
		lg:   lg,
		hack: make(map[string]storage.Lock),
	}, nil
}

var _ testgrpc.TestLockerServer = (*LockServer)(nil)

func (l *LockServer) Lock(ctx context.Context, req *testgrpc.LockRequest) (*testgrpc.LockResponse, error) {
	lg := l.lg.With("key", req.Key)
	if req.Key == "" {
		return nil, validation.Error("key required")
	}
	lg.Debug("acquiring blocking lock...")
	if req.Dur != nil && (req.Dur.AsDuration()) > 0 {
		ctxca, ca := context.WithTimeout(ctx, req.Dur.AsDuration())
		defer ca()
		ctx = ctxca
	}

	lock := l.lm.NewLock(req.Key)

	done, err := lock.Lock(ctx)
	if err != nil {
		lg.With("err", err.Error()).Error("lock likely held by someone else")
		return nil, err
	}

	l.hackMu.Lock()
	l.hack[req.Key] = lock
	l.hackMu.Unlock()

	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				lg.Info("holding")
			case <-done:
				lg.Warn("lock expired")
				return
			}
		}
	}()

	lg.Debug("sucessfully acquired lock")
	return &testgrpc.LockResponse{
		Acquired: true,
		Status:   "",
	}, nil
}

func (l *LockServer) TryLock(ctx context.Context, req *testgrpc.LockRequest) (*testgrpc.LockResponse, error) {
	lg := l.lg.With("key", req.Key)
	if req.Key == "" {
		return nil, validation.Error("key required")
	}
	lg.Debug("acquiring non-blocking lock...")
	if req.Dur != nil && (req.Dur.AsDuration()) > 0 {
		ctxca, ca := context.WithTimeout(ctx, req.Dur.AsDuration())
		defer ca()
		ctx = ctxca
	}

	lock := l.lm.NewLock(req.Key)

	ack, done, err := lock.TryLock(ctx)
	if err != nil {
		lg.Error(err.Error())
		return nil, err
	}

	if !ack {
		lg.Warn("lock is held by someone else")
		return &testgrpc.LockResponse{
			Acquired: false,
			Status:   "lock is held by someone else",
		}, nil
	}

	l.hackMu.Lock()
	l.hack[req.Key] = lock
	l.hackMu.Unlock()

	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				lg.Info("holding")
			case <-done:
				lg.Warn("lock expired")
				return
			}
		}
	}()
	l.lg.Info("successfully acquired lock")

	return &testgrpc.LockResponse{
		Acquired: ack,
		Status:   "",
	}, nil
}

func (l *LockServer) Unlock(ctx context.Context, req *testgrpc.UnlockRequest) (*testgrpc.UnlockResponse, error) {
	if req.Key == "" {
		return nil, validation.Error("key required")
	}

	lock, ok := l.hack[req.Key]
	if !ok {
		return nil, status.Error(codes.NotFound, "lock not tracked by server")
	}

	if err := lock.Unlock(); err != nil {
		l.lg.Error(err.Error())
		return nil, err
	}

	l.hackMu.Lock()
	delete(l.hack, req.Key)
	l.hackMu.Unlock()

	return &testgrpc.UnlockResponse{
		Unlocked: true,
	}, nil
}

func (l *LockServer) ListLocks(ctx context.Context, _ *emptypb.Empty) (*testgrpc.ListLocksResponse, error) {
	l.hackMu.Lock()
	keys := lo.Keys(l.hack)
	l.hackMu.Unlock()

	return &testgrpc.ListLocksResponse{
		Keys: keys,
	}, nil
}

type LockBackendConfig struct {
	Etcd      *v1beta1.EtcdStorageSpec
	Jetstream *v1beta1.JetStreamStorageSpec
}

type LockConfig struct {
	Key            string
	AcquireTimeout time.Duration
	ConfigPath     string
	Try            bool
}

func (l *LockConfig) Validate() error {
	if l.Key == "" {
		return validation.Error("key required")
	}
	if l.ConfigPath == "" {
		return validation.Error("lock config path is required")
	}
	return nil
}

func (l *LockBackendConfig) Validate() error {
	if l.Etcd == nil && l.Jetstream == nil {
		return validation.Error("must specify one lock backend")
	}

	if l.Etcd != nil && l.Jetstream != nil {
		return validation.Error("only one lock backend can be used at a time")
	}
	return nil
}

func setup(
	lg *slog.Logger,
	configPath string,
) (storage.LockManager, error) {
	lg.With("config", configPath).Info("Fetching lock server config")
	config, err := getConfig(configPath)
	if err != nil {
		lg.Error("failed to get config")
		return nil, err
	}

	// intentional : use background context so that clients do not trivially close their connections as soon cmd.Context() is signaled as done
	lm, err := getLockManager(context.Background(), config)
	if err != nil {
		configB, _ := json.Marshal(config)
		lg.With("config", string(configB)).Error(err.Error())
		return nil, err
	}
	return lm, nil
}

func getLockManager(ctx context.Context, config *LockBackendConfig) (storage.LockManager, error) {
	if config.Etcd != nil {
		client, err := etcd.NewEtcdClient(ctx, config.Etcd)
		if err != nil {
			return nil, err
		}
		lm := etcd.NewEtcdLockManager(client, "test/lock", logger.NewNop())
		return lm, nil
	}
	if config.Jetstream != nil {
		js, err := jetstream.AcquireJetstreamConn(context.Background(), config.Jetstream, logger.New().WithGroup("js"))
		if err != nil {
			return nil, err
		}

		lm := jetstream.NewLockManager(ctx, js, "test/lock", logger.NewNop())
		if err != nil {
			return nil, err
		}
		return lm, nil
	}
	return nil, fmt.Errorf("unsupported lock backend")
}

func getConfig(configPath string) (*LockBackendConfig, error) {
	if configPath == "" {
		return nil, validation.Error("config path required")
	}
	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rawConfig, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	config := &LockBackendConfig{}
	if err := json.Unmarshal(rawConfig, config); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}
