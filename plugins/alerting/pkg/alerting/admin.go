package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/alerting/storage/spec"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"gopkg.in/yaml.v2"

	alertingSync "github.com/rancher/opni/pkg/alerting/server/sync"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/metrics"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
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

	metricsExporter *SyncMetricsExporter
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
	for _, syncer := range s.syncPushers {
		syncer := syncer
		go func() {
			syncer <- req
		}()
	}
}

func NewSyncController() SyncController {
	return SyncController{
		syncPushers:     map[string]chan *alertops.SyncRequest{},
		syncMu:          &sync.RWMutex{},
		heartbeatTicker: time.NewTicker(SyncInterval),
		forceSyncTicker: time.NewTicker(ForceSyncInterval),
		metricsExporter: NewSyncMetricsExporter(),
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
	syncChan := make(chan *alertops.SyncRequest, 16)
	defer close(syncChan)
	p.syncController.AddSyncPusher(request.LifecycleUuid, syncChan)
	for {
		select {
		case <-p.ctx.Done():
			lg.Debug("exiting syncer loop, ops node shutting down")
			return nil
		case <-syncerServer.Context().Done():
			lg.Debug("exiting syncer loop, remote syncer shutting down")
			p.syncController.RemoveSyncPusher(request.LifecycleUuid)
			return nil
		case syncReq := <-syncChan:
			go func(req *alertops.SyncRequest) {
				err := syncerServer.Send(syncReq)
				if err != nil {
					lg.Errorf("could not send sync request to remote syncer %s", err)
				}
			}(syncReq)
		}
	}
}

func (p *Plugin) constructSyncRequest(
	ctx context.Context,
	routerKeys []string,
	routers spec.RouterStorage,
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

func (p *Plugin) doConfigSync(ctx context.Context, syncInfo alertingSync.SyncInfo) error {
	if !syncInfo.ShouldSync {
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

func (p *Plugin) doConfigForceSync(ctx context.Context, syncInfo alertingSync.SyncInfo) error {
	if !syncInfo.ShouldSync {
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

func (p *Plugin) getSyncInfo(ctx context.Context) (alertingSync.SyncInfo, error) {
	syncInfo := alertingSync.SyncInfo{
		ShouldSync: false,
		Clusters:   map[string]*corev1.Cluster{},
	}
	clStatus, err := p.GetClusterStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return syncInfo, err
	}
	syncInfo.ShouldSync = clStatus.State == alertops.InstallState_Installed

	mgmtClient, err := p.mgmtClient.GetContext(ctx)
	if err != nil {
		return syncInfo, err
	}
	cls, err := mgmtClient.ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		return syncInfo, err
	}

	syncInfo.Clusters = lo.Associate(cls.GetItems(), func(c *corev1.Cluster) (string, *corev1.Cluster) {
		return c.Id, c
	})
	return syncInfo, nil
}

func (p *Plugin) runSyncTasks(tasks []alertingSync.SyncTask) (retErr error) {
	lg := p.logger.With("action", "runSyncTasks")
	ctx, ca := context.WithTimeout(p.ctx, 10*time.Second)
	defer ca()
	start := time.Now()
	defer func() {
		cycleAttributes := []attribute.KeyValue{
			{
				Key:   "status",
				Value: attribute.StringValue("success"),
			},
		}
		if retErr != nil {
			cycleAttributes[0].Value = attribute.StringValue("failure")
		}
		p.syncController.metricsExporter.syncCycleStatusCounter.Add(ctx, 1, metric.WithAttributes(
			cycleAttributes...,
		))
		duration := time.Since(start).Milliseconds()
		p.syncController.metricsExporter.syncCycleProcessLatency.Record(ctx, duration, metric.WithAttributes(
			cycleAttributes...,
		))
	}()

	syncInfo, err := p.getSyncInfo(ctx)
	if err != nil {
		lg.Error("skipping alerting periodic sync due to error : %s", err)
		return err
	}

	lg.Info("Running periodic sync for alerting")
	var eg util.MultiErrGroup
	for _, task := range tasks {
		task := task
		eg.Go(func() error {
			return task(ctx, syncInfo)
		})
	}
	eg.Wait()
	lg.Infof("finished running periodic sync for alerting, sucessfully ran %d/%d sync tasks", len(tasks)-len(eg.Errors()), len(tasks))
	if err := eg.Error(); err != nil {
		lg.Error(err)
		retErr = err
	}
	return
}

func (p *Plugin) runSync() {
	ticker := p.syncController.heartbeatTicker
	longTicker := p.syncController.forceSyncTicker
	defer ticker.Stop()
	defer longTicker.Stop()
	syncTasks := []alertingSync.SyncTask{p.doConfigSync}
	forceSyncTasks := []alertingSync.SyncTask{p.doConfigForceSync}
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
			if err := p.runSyncTasks(syncTasks); err != nil {
				p.logger.Errorf("failed to successfully run all alerting sync tasks : %s", err)
			}
		case <-longTicker.C:
			if err := p.runSyncTasks(forceSyncTasks); err != nil {
				p.logger.Errorf("failed to successfully run all alerting force sync tasks : %s", err)
			}
		}
	}
}

func (p *Plugin) SendManualSyncRequest(
	ctx context.Context,
	routerKeys []string,
	routers spec.RouterStorage,
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

type SyncMetricsExporter struct {
	syncCycleProcessLatency metric.Int64Histogram
	syncCycleStatusCounter  metric.Int64Counter
}

func NewSyncMetricsExporter() *SyncMetricsExporter {
	return &SyncMetricsExporter{
		syncCycleProcessLatency: metrics.SyncCycleProcessLatency,
		syncCycleStatusCounter:  metrics.SyncCycleStatusCounter,
	}
}
