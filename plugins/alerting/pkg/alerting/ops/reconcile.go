package ops

import (
	"context"
	"fmt"
	"time"

	"github.com/rancher/opni/pkg/alerting/storage"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v2"
)

func (a *AlertingOpsNode) ConnectRemoteSyncer(
	req *alertops.ConnectRequest,
	remoteServer alertops.ConfigReconciler_ConnectRemoteSyncerServer,
) error {
	lg := a.logger.With("method", "ConnectRemoteSyncer", "syncer", req.LifecycleUuid)
	ctxTimeout, ca := context.WithTimeout(a.ctx, a.storageTimeout)
	defer ca()
	storageClientSet, err := a.storageClientSet.GetContext(ctxTimeout)
	if err != nil {
		return status.Error(codes.Unavailable, fmt.Sprintf("failed to get storage client set: %s", err))
	}
	routerKeys, err := storageClientSet.Routers().ListKeys(a.ctx)
	if err != nil {
		return status.Error(codes.Internal, "failed to list router keys")
	}
	lg.Infof("connected remote syncer")

	lg.Debug("performing initial sync")
	syncReq := a.constructSyncRequest(a.ctx, routerKeys, storageClientSet.Routers())
	err = remoteServer.Send(syncReq)
	if err != nil {
		a.logger.Error("failed to send initial sync")
	}
	lg.Debug("finished performing intial sync")

	for {
		select {
		case <-a.ctx.Done():
			lg.Debug("exiting syncer loop, ops node shutting down")
			return nil
		case <-remoteServer.Context().Done():
			lg.Debug("exiting syncer loop, remote syncer shutting down")
			return nil
		case syncReq := <-a.syncPusher:
			err := remoteServer.Send(syncReq)
			if err != nil {
				lg.Errorf("could not send sync request to remote syncer %s", err)
			}
		}
	}
}

func (a *AlertingOpsNode) runPeriodicSync(ctx context.Context) {
	lg := a.logger.With("method", "runPeriodicSync")
	ticker := time.NewTicker(time.Minute * 1)
	longTicker := time.NewTicker(time.Minute * 15)
	defer ticker.Stop()
	defer longTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.logger.Debug("exiting periodic sync loop")
			return
		case <-ticker.C:
			status, err := a.GetClusterStatus(ctx, &emptypb.Empty{})
			if err != nil {
				lg.Warnf("skipping periodic sync due to status error: %s", err)
				continue
			}
			if status.State != alertops.InstallState_Installed {
				continue
			}
			lg.Info("Running periodic sync for alerting")
			go func() {
				a.syncMu.Lock()
				defer a.syncMu.Unlock()
				ctxTimeout, ca := context.WithTimeout(ctx, a.storageTimeout)
				defer ca()
				clientSet, err := a.storageClientSet.GetContext(ctxTimeout)
				if err != nil {
					lg.Warn("failed to acquire alerting storage clientset, skipping sync...")
					return
				}
				routerKeys, err := clientSet.Sync(ctx)
				if err != nil {
					lg.Errorf("failed to sync configuration in alerting clientset %s", err)
				}
				if len(routerKeys) > 0 {
					lg.Debug("sync change detected, pushing sync request to remote syncers")
					syncReq := a.constructSyncRequest(ctx, routerKeys, clientSet.Routers())
					a.syncPusher <- syncReq
				}
			}()
		case <-longTicker.C:
			lg.Info("Running long periodic sync for alerting")
			// forcibly recalculates the hash
			go func() {
				a.syncMu.Lock()
				defer a.syncMu.Unlock()
				ctxTimeout, ca := context.WithTimeout(ctx, a.storageTimeout)
				defer ca()
				clientSet, err := a.storageClientSet.GetContext(ctxTimeout)
				if err != nil {
					lg.Warn("failed to acquire alerting storage clientset, skipping force sync...")
					return
				}
				if err := clientSet.ForceSync(ctx); err != nil {
					lg.Errorf("failed to force sync configuration in alerting clientset %s", err)
					return
				}
				routers := clientSet.Routers()
				routerKeys, err := routers.ListKeys(ctx)
				if err != nil {
					lg.Errorf("failed to get router keys during force sync %s", err)
					return
				}
				syncReq := a.constructSyncRequest(ctx, routerKeys, routers)
				a.syncPusher <- syncReq
			}()
		}
	}
}

// routerKeys correspond to the global key or tenant-ed keys
func (a *AlertingOpsNode) SendManualSyncRequest(
	ctx context.Context,
	routerKeys []string,
	routers storage.RouterStorage,
) {
	a.syncMu.Lock()
	defer a.syncMu.Unlock()

	lg := a.logger.With("method", "sendManualSyncRequest")
	syncReq := a.constructSyncRequest(ctx, routerKeys, routers)
	a.syncPusher <- syncReq
	lg.Debug("sent manual sync request")
}

func (a *AlertingOpsNode) constructSyncRequest(
	ctx context.Context,
	routerKeys []string,
	routers storage.RouterStorage,
) *alertops.SyncRequest {
	lg := a.logger.With("method", "constructSyncRequest")
	syncReq := &alertops.SyncRequest{
		Items: []*alertingv1.PutConfigRequest{},
	}

	for _, key := range routerKeys {
		router, err := routers.Get(a.ctx, key)
		if err != nil {
			lg.Errorf("failed to get router %s from storage: %s", key, err)
			continue
		}
		config, err := router.BuildConfig()
		if err != nil {
			lg.Errorf("failed to build config for router %s: %s", key, err)
			continue
		}

		data, err := yaml.Marshal(config)
		if err != nil {
			lg.Errorf("failed to marshal config for router %s: %s", key, err)
			continue
		}

		syncReq.Items = append(syncReq.Items, &alertingv1.PutConfigRequest{
			Key:    key,
			Config: data,
		})
	}
	return syncReq
}
