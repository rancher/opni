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

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"gopkg.in/yaml.v2"
)

type AlertManagerSyncerV1 struct {
	util.Initializer
	alertingv1.UnsafeSyncerServer

	lg           *zap.SugaredLogger
	uuid         string
	serverConfig *alertingv1.SyncerConfig
	lastSynced   *timestamppb.Timestamp

	alertingClient     client.Client
	gatewayClient      alertops.ConfigReconcilerClient
	remoteSyncerClient alertops.ConfigReconciler_ConnectRemoteSyncerClient
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
		lg:           logger.NewPluginLogger().Named("alerting-syncer"),
		uuid:         uuid.New().String(),
	}
	go func() {
		server.lg.Infof("starting alerting syncer server as \"%s\"...", server.uuid)
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
		a.alertingClient = client.NewClient(
			nil,
			a.serverConfig.AlertmanagerAddress,
			a.serverConfig.HookListenAddress,
		)
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

func (a *AlertManagerSyncerV1) connect(ctx context.Context) {
	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(0),
		backoffv2.WithMinInterval(5*time.Second),
		backoffv2.WithMaxInterval(10*time.Minute),
		backoffv2.WithMultiplier(1.5),
	)
	syncer := retrier.Start(ctx)
	var syncerClient alertops.ConfigReconciler_ConnectRemoteSyncerClient
	for backoffv2.Continue(syncer) {
		client, err := a.gatewayClient.ConnectRemoteSyncer(ctx, &alertops.ConnectRequest{
			LifecycleUuid: a.uuid,
		})
		if err == nil {
			syncerClient = client
			break
		} else {
			a.lg.Errorf("failed to connect to remote syncer: %s", err)
		}
	}
	a.remoteSyncerClient = syncerClient
	a.lg.Debug("connected to remote syncer")
}

func (a *AlertManagerSyncerV1) recvMsgs(
	ctx context.Context,
	cc *grpc.ClientConn,
) {
	defer cc.Close()
	reconnectDur := 1 * time.Minute
	reconnectTimer := time.NewTimer(time.Second)

	for { // connect loop
		select {
		case <-ctx.Done():
			return
		case <-reconnectTimer.C:
			a.lg.Debug("starting new stream subscription")

			for { // recv loop
				syncReq, err := a.remoteSyncerClient.Recv()
				if err == io.EOF {
					a.lg.Warnf("remote syncer disconnected, reconnecting, ...")
					a.connect(ctx)
					break
				}

				if st, ok := status.FromError(err); ok && err != nil {
					if st.Code() == codes.Unimplemented {
						panic(err)
					} else if st.Code() == codes.Unavailable {
						a.lg.Warnf("remote syncer unavailable, reconnecting, ...")
						a.connect(ctx)
						break
					} else {
						a.lg.With("code", st.Code()).Errorf("failed to receive sync config message: %s", err)
						// need to setup a new stream subscription
						break
					}

				} else if err != nil {
					// need to setup a new stream subscription
					a.lg.Errorf("failed to receive sync config message: %s", err)
					break
				}
				a.lg.Info("received sync config message")
				for _, req := range syncReq.GetItems() {
					if _, err := a.PutConfig(ctx, req); err != nil {
						a.lg.Errorf("failed to put config: %s", err)
					}
				}
			}
			reconnectTimer.Reset(reconnectDur)
		}
	}
}

// func (a *AlertManagerSyncerV1) processMsg

func (a *AlertManagerSyncerV1) PutConfig(ctx context.Context, incomingConfig *alertingv1.PutConfigRequest) (*emptypb.Empty, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "alerting syncer is not ready")
	}
	var c *config.Config
	if err := yaml.Unmarshal(incomingConfig.Config, &c); err != nil {
		return nil, validation.Errorf("improperly formatted config : %s", err)
	}
	if err := os.WriteFile(a.serverConfig.AlertmanagerConfigPath, incomingConfig.GetConfig(), 0644); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to write config to file: %s", err))
	}
	lg := a.lg.With("syncer-id", a.uuid)
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
		err := a.alertingClient.Ready(ctx)
		if err != nil {
			lg.Warnf("alerting client not yet ready: %s", err)
			continue
		}

		err = a.alertingClient.Reload(ctx)
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
