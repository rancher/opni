package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/alerting/storage"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/util"

	"github.com/rancher/opni/plugins/alerting/pkg/alerting/metrics"
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

type SyncController struct {
	syncPushers     map[string]chan *alertops.SyncRequest
	syncMu          *sync.RWMutex
	heartbeatTicker *time.Ticker
	forceSyncTicker *time.Ticker
}

func (s *SyncController) AddSyncPusher(LifecycleUuid string, pusher chan *alertops.SyncRequest) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.syncPushers[LifecycleUuid] = pusher
}

func (s *SyncController) RemoveSyncPusher(LifecycleUuid string) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	delete(s.syncPushers, LifecycleUuid)
}

func (s *SyncController) PushSyncReq(req *alertops.SyncRequest) {
	for _, syncers := range s.syncPushers {
		syncers <- req
	}
}

func NewSyncController() SyncController {
	return SyncController{
		syncPushers:     map[string]chan *alertops.SyncRequest{},
		syncMu:          &sync.RWMutex{},
		heartbeatTicker: time.NewTicker(SyncInterval),
		forceSyncTicker: time.NewTicker(ForceSyncInterval),
	}
}

var (
	_ alertops.ConfigReconcilerServer = (*Plugin)(nil)
	_ alertops.AlertingAdminServer    = (*Plugin)(nil)
)

func (p *Plugin) GetClusterConfiguration(ctx context.Context, _ *emptypb.Empty) (*alertops.ClusterConfiguration, error) {
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

func (p *Plugin) GetClusterStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.InstallStatus, error) {
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

func (p *Plugin) InstallCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
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
	lg.Infof("connected remote syncer, performaing initial sync...")
	syncReq := p.constructSyncRequest(p.ctx, routerKeys, storageClientSet.Routers())
	err = syncerServer.Send(syncReq)
	if err != nil {
		lg.Error("failed to send initial sync")
	}
	lg.Debug("finished performing intial sync")
	syncChan := make(chan *alertops.SyncRequest)
	p.syncController.AddSyncPusher(request.LifecycleUuid, syncChan)
	for {
		select {
		case <-p.ctx.Done():
			lg.Debug("exiting syncer loop, ops node shutting down")
			return nil
		case <-syncerServer.Context().Done():
			lg.Debug("exiting syncer loop, remote syncer shutting down")
			return nil
		case syncReq := <-syncChan:
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

func (p *Plugin) doConfigSync(ctx context.Context, shouldSync bool) error {
	if !shouldSync {
		return nil
	}
	lg := p.logger.With("method", "doSync")
	p.syncController.syncMu.Lock()
	defer p.syncController.syncMu.Unlock()
	ctxTimeout, ca := context.WithTimeout(ctx, 5*time.Second)
	defer ca()
	clientSet, err := p.storageClientSet.GetContext(ctxTimeout)
	if err != nil {
		lg.Warn("failed to acquire alerting storage clientset, skipping sync...")
		return err
	}
	routerKeys, err := clientSet.Sync(ctx)
	if err != nil {
		lg.Errorf("failed to sync configuration in alerting clientset %s", err)
		return err
	}
	if len(routerKeys) > 0 {
		lg.Debug("sync change detected, pushing sync request to remote syncers")
		syncReq := p.constructSyncRequest(ctx, routerKeys, clientSet.Routers())
		p.syncController.PushSyncReq(syncReq)
	}
	return nil
}

func (p *Plugin) doConfigForceSync(ctx context.Context, shouldSync bool) error {
	if !shouldSync {
		return nil
	}
	lg := p.logger.With("method", "doForceSync")
	p.syncController.syncMu.Lock()
	defer p.syncController.syncMu.Unlock()
	ctxTimeout, ca := context.WithTimeout(ctx, 5*time.Second)
	defer ca()
	clientSet, err := p.storageClientSet.GetContext(ctxTimeout)
	if err != nil {
		lg.Warn("failed to acquire alerting storage clientset, skipping force sync...")
		return err
	}
	if err := clientSet.ForceSync(ctx); err != nil {
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
	p.syncController.PushSyncReq(syncReq)
	return nil
}

type syncTask func(ctx context.Context, shouldSync bool) error

func (p *Plugin) runSyncTasks(tasks []syncTask) (retErr error) {
	lg := p.logger.With("action", "runSyncTasks")
	start := time.Now()
	defer func() {
		if retErr != nil {
			metrics.SyncCycleFailedCounter.Inc()
		}
		metrics.SyncCycleCounter.Inc()
		duration := time.Since(start).Milliseconds()
		metrics.SyncCycleProcessLatency.Observe(float64(duration))
	}()
	clStatus, err := p.GetClusterStatus(p.ctx, &emptypb.Empty{})
	if err != nil {
		lg.Warnf("skipping periodic sync due to status error: %s", err)
		retErr = err
		return
	}
	shouldSync := clStatus.State == alertops.InstallState_Installed
	lg.Info("Running periodic sync for alerting")
	var eg util.MultiErrGroup
	for _, task := range tasks {
		task := task
		eg.Go(func() error {
			return task(p.ctx, shouldSync)
		})
	}
	eg.Wait()
	if err := eg.Error(); err != nil {
		retErr = err
	}
	return
}

func (p *Plugin) runSync() {
	ticker := p.syncController.heartbeatTicker
	longTicker := p.syncController.forceSyncTicker
	defer ticker.Stop()
	defer longTicker.Stop()
	syncTasks := []syncTask{p.doConfigSync}
	forceSyncTasks := []syncTask{p.doConfigForceSync}
	for _, comp := range p.Components() {
		syncTasks = append(syncTasks, comp.Sync)
		forceSyncTasks = append(forceSyncTasks, comp.Sync)
	}
	for {
		select {
		case <-p.ctx.Done():
			p.logger.Info("exiting main sync loop")
			return
		case <-ticker.C:
			p.runSyncTasks(syncTasks)
		case <-longTicker.C:
			p.runSyncTasks(forceSyncTasks)
		}
	}
}

func (p *Plugin) SendManualSyncRequest(
	ctx context.Context,
	routerKeys []string,
	routers storage.RouterStorage,
) {
	p.syncController.syncMu.Lock()
	defer p.syncController.syncMu.Unlock()

	lg := p.logger.With("method", "sendManualSyncRequest")
	syncReq := p.constructSyncRequest(ctx, routerKeys, routers)
	p.syncController.PushSyncReq(syncReq)
	lg.Debug("sent manual sync request")
}

func (p *Plugin) ready() error {
	for _, comp := range p.Components() {
		if !comp.Ready() {
			return fmt.Errorf("alerting server component '%s' is not intialized yet", comp.Name())
		}
	}
	return nil
}

func (p *Plugin) healthy() error {
	for _, comp := range p.Components() {
		if !comp.Healthy() {
			return fmt.Errorf("alerting server component '%s' is not intialized yet", comp.Name())
		}
	}
	return nil
}
