package alerting

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage"
	sync_opts "github.com/rancher/opni/pkg/alerting/storage/opts"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/util"

	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v2"
)

var (
	SyncInterval      = time.Minute * 1
	ForceSyncInterval = time.Minute * 15
)

type SyncConfig struct {
	syncPusher      chan *alertops.SyncRequest
	syncMu          *sync.RWMutex
	heartbeatTicker *time.Ticker
	forceSyncTicker *time.Ticker
}

func NewSyncConfig() SyncConfig {
	return SyncConfig{
		syncPusher:      make(chan *alertops.SyncRequest),
		syncMu:          &sync.RWMutex{},
		heartbeatTicker: time.NewTicker(SyncInterval),
		forceSyncTicker: time.NewTicker(ForceSyncInterval),
	}
}

type Syncer interface {
	alertops.ConfigReconcilerServer
	SendManualSyncRequest()
	CollectAndReconcile()
	Sync()
	ForceSync()
}

var _ alertops.ConfigReconcilerServer = (*Plugin)(nil)
var _ alertops.AlertingAdminServer = (*Plugin)(nil)

func (p *Plugin) GetClusterConfiguration(ctx context.Context, empty *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
	ctxTimeout, ca := context.WithTimeout(ctx, 1*time.Second)
	defer ca()
	clDriver, err := p.clusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	return clDriver.GetClusterConfiguration(ctx, &emptypb.Empty{})
}

func (p *Plugin) ConfigureCluster(ctx context.Context, configuration *alertops.ClusterConfiguration) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	driver, err := p.clusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.ConfigureCluster(ctx, configuration)
}

func (p *Plugin) GetClusterStatus(ctx context.Context, empty *emptypb.Empty) (*alertops.InstallStatus, error) {
	ctxTimeout, ca := context.WithTimeout(ctx, 1*time.Second)
	defer ca()
	clDriver, err := p.clusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	retStatus, err := clDriver.GetClusterStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	for _, comp := range p.Components() {
		if !comp.Ready() {
			retStatus.Conditions = append(retStatus.Conditions, fmt.Sprintf("%s server is not yet ready", comp.Name()))
			continue
		}
		if !comp.Healthy() {
			retStatus.Conditions = append(retStatus.Conditions, fmt.Sprintf("%s server is unhealthy", comp.Name()))
		}
	}
	return retStatus, nil
}

func (p *Plugin) InstallCluster(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	driver, err := p.clusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return driver.InstallCluster(ctx, &emptypb.Empty{})
}

func (p *Plugin) UninstallCluster(ctx context.Context, request *alertops.UninstallRequest) (*emptypb.Empty, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	driver, err := p.clusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	if request.DeleteData {
		go func() {
			err := p.storageClientSet.Get().Purge(context.Background())
			if err != nil {
				p.logger.Warnf("failed to purge data %s", err)
			}
		}()
	}
	return driver.UninstallCluster(ctx, request)
}

func (p *Plugin) ConnectRemoteSyncer(request *alertops.ConnectRequest, syncerServer alertops.ConfigReconciler_ConnectRemoteSyncerServer) error {
	lg := p.logger.With("method", "ConnectRemoteSyncer", "syncer", request.LifecycleUuid)
	ctxTimeout, ca := context.WithTimeout(p.ctx, time.Second*5)
	defer ca()
	storageClientSet, err := p.storageClientSet.GetContext(ctxTimeout)
	if err != nil {
		return status.Error(codes.Unavailable, fmt.Sprintf("failed to get storage client set: %s", err))
	}
	routerKeys, err := storageClientSet.Routers().ListKeys(p.ctx)
	if err != nil {
		return status.Error(codes.Internal, "failed to list router keys")
	}
	lg.Infof("connected remote syncer")

	lg.Debug("performing initial sync")
	syncReq := p.constructSyncRequest(p.ctx, routerKeys, storageClientSet.Routers())
	err = syncerServer.Send(syncReq)
	if err != nil {
		lg.Error("failed to send initial sync")
	}
	lg.Debug("finished performing intial sync")

	for {
		select {
		case <-p.ctx.Done():
			lg.Debug("exiting syncer loop, ops node shutting down")
			return nil
		case <-syncerServer.Context().Done():
			lg.Debug("exiting syncer loop, remote syncer shutting down")
			return nil
		case syncReq := <-p.syncConfig.syncPusher:
			err := syncerServer.Send(syncReq)
			if err != nil {
				lg.Errorf("could not send sync request to remote syncer %s", err)
			}
		}
	}
}

func (p *Plugin) constructSyncRequest(
	ctx context.Context,
	routerKeys []string,
	routers storage.RouterStorage,
) *alertops.SyncRequest {
	lg := p.logger.With("method", "constructSyncRequest")
	syncReq := &alertops.SyncRequest{
		Items: []*alertingv1.PutConfigRequest{},
	}

	for _, key := range routerKeys {
		router, err := routers.Get(ctx, key)
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

func (p *Plugin) doSync(ctx context.Context, shouldSync bool) error {
	if !shouldSync {
		return nil
	}
	lg := p.logger.With("method", "doSync")
	p.syncConfig.syncMu.Lock()
	defer p.syncConfig.syncMu.Unlock()
	ctxTimeout, ca := context.WithTimeout(ctx, 5*time.Second)
	defer ca()
	clientSet, err := p.storageClientSet.GetContext(ctxTimeout)
	if err != nil {
		lg.Warn("failed to acquire alerting storage clientset, skipping sync...")
		return err
	}
	routerKeys, err := clientSet.Sync(ctx, sync_opts.WithDefaultReceiverAddreess(
		util.Must(
			url.Parse(path.Join("http://localhost:3000", shared.AlertingDefaultHookName)),
		),
	))
	if err != nil {
		lg.Errorf("failed to sync configuration in alerting clientset %s", err)
		return err
	}
	if len(routerKeys) > 0 {
		lg.Debug("sync change detected, pushing sync request to remote syncers")
		syncReq := p.constructSyncRequest(ctx, routerKeys, clientSet.Routers())
		p.syncConfig.syncPusher <- syncReq
	}
	return nil
}

func (p *Plugin) doForceSync(ctx context.Context, shouldSync bool) error {
	if !shouldSync {
		return nil
	}
	lg := p.logger.With("method", "doForceSync")
	p.syncConfig.syncMu.Lock()
	defer p.syncConfig.syncMu.Unlock()
	ctxTimeout, ca := context.WithTimeout(ctx, 5*time.Second)
	defer ca()
	clientSet, err := p.storageClientSet.GetContext(ctxTimeout)
	if err != nil {
		lg.Warn("failed to acquire alerting storage clientset, skipping force sync...")
		return err
	}
	if err := clientSet.ForceSync(ctx, sync_opts.WithDefaultReceiverAddreess(
		util.Must(
			url.Parse(path.Join("http://localhost:3000", shared.AlertingDefaultHookName)),
		),
	)); err != nil {
		lg.Errorf("failed to force sync configuration in alerting clientset %s", err)
		return err
	}
	routers := clientSet.Routers()
	routerKeys, err := routers.ListKeys(ctx)
	if err != nil {
		lg.Errorf("failed to get router keys during force sync %s", err)
		return err
	}
	syncReq := p.constructSyncRequest(p.ctx, routerKeys, routers)
	p.syncConfig.syncPusher <- syncReq
	return nil
}

type syncTask func(ctx context.Context, shouldSync bool) error

func (p *Plugin) runSync() {
	lg := p.logger.With("method", "runPeriodicSync")
	ticker := p.syncConfig.heartbeatTicker
	longTicker := p.syncConfig.forceSyncTicker
	defer ticker.Stop()
	defer longTicker.Stop()
	syncTasks := []syncTask{p.doSync}
	forceSyncTasks := []syncTask{p.doForceSync}
	for _, comp := range p.Components() {
		syncTasks = append(syncTasks, comp.Sync)
		forceSyncTasks = append(forceSyncTasks, comp.Sync)
	}
	for {
		select {
		case <-p.ctx.Done():
			lg.Debug("exiting main sync loop")
			return
		case <-ticker.C:
			clStatus, err := p.GetClusterStatus(p.ctx, &emptypb.Empty{})
			if err != nil {
				lg.Warnf("skipping periodic sync due to status error: %s", err)
				continue
			}
			shouldSync := clStatus.State == alertops.InstallState_Installed
			lg.Info("Running periodic sync for alerting")
			var eg util.MultiErrGroup
			eg.Add(len(forceSyncTasks))
			for _, task := range forceSyncTasks {
				task := task
				eg.Go(func() error {
					return task(p.ctx, shouldSync)
				})
			}
			eg.Wait()
			if err := eg.Error(); err != nil {
				lg.Errorf("force sync request failed %s", err)
			}
		case <-longTicker.C:
			lg.Info("Running long periodic sync for alerting")
			clStatus, err := p.GetClusterStatus(p.ctx, &emptypb.Empty{})
			if err != nil {
				lg.Warnf("skipping periodic sync due to status error: %s", err)
				continue
			}
			shouldSync := clStatus.State == alertops.InstallState_Installed
			var eg util.MultiErrGroup
			eg.Add(len(forceSyncTasks))
			for _, task := range forceSyncTasks {
				task := task
				eg.Go(func() error {
					return task(p.ctx, shouldSync)
				})
			}
			eg.Wait()
			if err := eg.Error(); err != nil {
				lg.Errorf("force sync request failed %s", err)
			}
		}
	}
}

func (p *Plugin) SendManualSyncRequest(
	ctx context.Context,
	routerKeys []string,
	routers storage.RouterStorage,
) {
	p.syncConfig.syncMu.Lock()
	defer p.syncConfig.syncMu.Unlock()

	lg := p.logger.With("method", "sendManualSyncRequest")
	syncReq := p.constructSyncRequest(ctx, routerKeys, routers)
	p.syncConfig.syncPusher <- syncReq
	lg.Debug("sent manual sync request")
}
