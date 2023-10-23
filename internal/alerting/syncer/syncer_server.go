package syncer

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rancher/opni/plugins/alerting/apis/alertops" //nolint:opni

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/clients"

	"log/slog"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"gopkg.in/yaml.v2"
)

type AlertManagerSyncerV1 struct {
	util.Initializer
	alertingv1.UnsafeSyncerServer

	lg           *slog.Logger
	serverConfig *alertingv1.SyncerConfig
	lastSynced   *timestamppb.Timestamp

	alertingClient client.AlertingClient
	gatewayClient  alertops.ConfigReconcilerClient
	whoami         string

	lastSyncId string
}

var _ alertingv1.SyncerServer = &AlertManagerSyncerV1{}

// requires access to the alerting storage node requirements
func NewAlertingSyncerV1(
	ctx context.Context,
	serverConfig *alertingv1.SyncerConfig,
	mgmtClient managementv1.ManagementClient,
) alertingv1.SyncerServer {
	init := &atomic.Bool{}
	init.Store(false)
	server := &AlertManagerSyncerV1{
		serverConfig: serverConfig,
		lg:           logger.NewPluginLogger().WithGroup("alerting-syncer"),
	}
	go func() {
		server.Initialize(ctx, mgmtClient)
	}()
	return server
}

func (a *AlertManagerSyncerV1) Initialize(
	ctx context.Context,
	mgmtClient managementv1.ManagementClient,
) {
	var gatewayCc *grpc.ClientConn
	a.InitOnce(func() {
		whoami := ""
		if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
			whoami += ns
		}
		if podName := os.Getenv("POD_NAME"); podName != "" {
			if len(whoami) > 0 {
				whoami += "/"
			}
			whoami += podName
		}
		if len(whoami) == 0 {
			whoami = uuid.New().String()
		}
		a.whoami = whoami
		a.lg.Info(fmt.Sprintf("starting alerting syncer server as identity %s", a.whoami))
		var initErr error
		a.alertingClient, initErr = client.NewClient(
			client.WithAlertManagerAddress(
				a.serverConfig.AlertmanagerAddress,
			),
			client.WithQuerierAddress(
				a.serverConfig.HookListenAddress,
			),
		)
		if initErr != nil {
			panic(initErr)
		}
		gatewayClient, err := clients.FromExtension(
			ctx,
			mgmtClient,
			"ConfigReconciler",
			alertops.NewConfigReconcilerClient,
		)
		if err != nil {
			panic(err)
		}

		a.gatewayClient = gatewayClient
		a.lg.Debug("acquired gateway alerting client")
		a.connect(ctx)
	})
	go a.recvMsgs(ctx, gatewayCc)
}

func (a *AlertManagerSyncerV1) connect(ctx context.Context) alertops.ConfigReconciler_SyncConfigClient {
	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(0),
		backoffv2.WithMinInterval(5*time.Second),
		backoffv2.WithMaxInterval(10*time.Minute),
		backoffv2.WithMultiplier(1.5),
	)
	syncer := retrier.Start(ctx)
	var syncerClient alertops.ConfigReconciler_SyncConfigClient
	for backoffv2.Continue(syncer) {
		a.lg.Debug("trying to acquire remote syncer stream")
		stream, err := a.gatewayClient.SyncConfig(ctx)
		if err != nil {
			a.lg.Error(fmt.Sprintf("failed to connect to gateway: %s", err))
			continue
		}
		syncerClient = stream
		break
	}
	a.lg.Debug("connected to remote syncer")
	return syncerClient
}

func (a *AlertManagerSyncerV1) recvMsgs(
	ctx context.Context,
	cc *grpc.ClientConn,
) {
	defer cc.Close()
	reconnectDur := 1 * time.Minute
	reconnectTimer := time.NewTimer(time.Millisecond)
	a.lg.Debug("starting alerting syncer stream subscription")
	for { // connect loop
		select {
		case <-ctx.Done():
			return
		case <-reconnectTimer.C:
			a.lg.Debug("starting new stream subscription")
			remoteSyncerClient := a.connect(ctx)
			for { // recv loop
			RECV:
				syncReq, err := remoteSyncerClient.Recv()
				if err == io.EOF {
					a.lg.Info("remote syncer disconnected, reconnecting, ...")
					break
				}
				if st, ok := status.FromError(err); ok && err != nil {
					if st.Code() == codes.Unimplemented {
						panic(err)
					} else if st.Code() == codes.Unavailable {
						a.lg.Warn("remote syncer unavailable, reconnecting, ...")
						break
					} else {
						a.lg.Error(fmt.Sprintf("failed to receive sync config message: %s", err), "code", st.Code())
						break
					}
				} else if err != nil {
					a.lg.Error(fmt.Sprintf("failed to receive sync config message: %s", err))
					break
				}
				a.lg.Debug(fmt.Sprintf("received sync (%s) config message", syncReq.SyncId))
				if a.lastSyncId == syncReq.SyncId {
					a.lg.Debug("already up to date")
					goto RECV
				}
				syncState := alertops.SyncState_Synced
				for _, req := range syncReq.GetItems() {
					if _, err := a.PutConfig(ctx, req); err != nil {
						a.lg.Error(fmt.Sprintf("failed to put config: %s", err))
						syncState = alertops.SyncState_SyncError
					}
				}
				a.lastSynced = timestamppb.Now()
				if syncState == alertops.SyncState_Synced {
					a.lastSyncId = syncReq.SyncId
				}
				if err := remoteSyncerClient.Send(&alertops.ConnectInfo{
					LifecycleUuid: syncReq.LifecycleId,
					Whoami:        a.whoami,
					State:         syncState,
					SyncId:        syncReq.SyncId,
				}); err != nil {
					a.lg.Error(fmt.Sprintf("failed to send sync state: %s", err))
				}
			}
			// close current stream & reconnect
			if err := remoteSyncerClient.CloseSend(); err != nil {
				a.lg.Error(fmt.Sprintf("failed to close stream: %s", err))
			}
			reconnectTimer.Reset(reconnectDur)
		}
	}
}

func (a *AlertManagerSyncerV1) PutConfig(ctx context.Context, incomingConfig *alertingv1.PutConfigRequest) (*emptypb.Empty, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "alerting syncer is not ready")
	}
	lg := a.lg.With("config-path", a.serverConfig.AlertmanagerConfigPath)
	var c *config.Config
	if err := yaml.Unmarshal(incomingConfig.Config, &c); err != nil {
		lg.Error(fmt.Sprintf("failed to unmarshal config: %s", err))
		return nil, validation.Errorf("improperly formatted config : %s", err)
	}
	if err := os.WriteFile(a.serverConfig.AlertmanagerConfigPath, incomingConfig.GetConfig(), 0644); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to write config to file: %s", err))
	}
	lg.Debug("put config request received")

	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(5),
		backoffv2.WithMinInterval(1*time.Second),
		backoffv2.WithMaxInterval(5*time.Minute),
		backoffv2.WithMultiplier(1.5),
	)
	b := retrier.Start(ctx)
	success := false
	for backoffv2.Continue(b) {
		err := a.alertingClient.StatusClient().Ready(ctx)
		if err != nil {
			lg.Warn(fmt.Sprintf("alerting client not yet ready: %s", err))
			continue
		}

		err = a.alertingClient.ControlClient().Reload(ctx)
		if err == nil {
			success = true
			break
		}
	}
	if !success {
		return nil, status.Error(codes.Internal, "failed to reload alertmanager config")
	}
	lg.Debug("config reloaded")
	return &emptypb.Empty{}, nil
}

func (a *AlertManagerSyncerV1) Ready(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "alerting syncer is not initialized")
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertManagerSyncerV1) Healthy(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "alerting syncer is not initialized")
	}
	return &emptypb.Empty{}, nil
}

func (a *AlertManagerSyncerV1) Status(ctx context.Context, _ *emptypb.Empty) (*alertingv1.SyncerStatus, error) {
	res := &alertingv1.SyncerStatus{
		Configs: make(map[string]string),
	}
	healthy := func() bool {
		_, err := a.Healthy(ctx, &emptypb.Empty{})
		return err == nil
	}
	ready := func() bool {
		_, err := a.Ready(ctx, &emptypb.Empty{})
		return err == nil
	}
	res.Healthy = healthy()
	res.Ready = ready()
	res.LastSynced = a.lastSynced
	return res, nil
}
