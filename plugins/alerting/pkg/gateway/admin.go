package alerting

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage/spec"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	alertingSync "github.com/rancher/opni/pkg/alerting/server/sync"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/gateway/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/gateway/metrics"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	SyncInterval      = time.Minute * 1
	ForceSyncInterval = time.Minute * 15
)

type RemoteInfo struct {
	WhoAmI        string
	LastApplied   *timestamppb.Timestamp
	LastSyncState alertops.SyncState
	LastSyncId    string
}

type SyncController struct {
	lg *zap.SugaredLogger

	hashMu          sync.Mutex
	syncMu          sync.RWMutex
	remoteMu        sync.RWMutex
	heartbeatTicker *time.Ticker
	forceSyncTicker *time.Ticker

	// lifecycleUUid -> vals
	syncPushers map[string]chan *alertops.SyncRequest
	remoteInfo  map[string]RemoteInfo

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

func (s *SyncController) AddRemoteInfo(LifecycleUuid string, info RemoteInfo) {
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	s.remoteInfo[LifecycleUuid] = info
}

func (s *SyncController) RemoveRemoteInfo(LifecycleUuid string) {
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	delete(s.remoteInfo, LifecycleUuid)
}

func (s *SyncController) ListRemoteInfo() map[string]RemoteInfo {
	s.remoteMu.RLock()
	defer s.remoteMu.RUnlock()
	copyM := map[string]RemoteInfo{}
	for id, info := range s.remoteInfo {
		copyM[id] = info
	}
	return copyM
}

func (s *SyncController) PushSyncReq(payload *syncPayload) {
	for id, syncer := range s.syncPushers {
		id := id
		syncer := syncer
		select {
		case syncer <- &alertops.SyncRequest{
			LifecycleId: id,
			SyncId:      payload.syncId,
			Items: []*alertingv1.PutConfigRequest{
				{
					Key:    payload.configKey,
					Config: payload.data,
				},
			},
		}:
		default:
			s.lg.With("syncer-id", id).Error("failed to push sync request : buffer already full")
		}
	}
}

func (s *SyncController) PushOne(lifecycleId string, payload *syncPayload) {
	if _, ok := s.syncPushers[lifecycleId]; ok {
		select {
		case s.syncPushers[lifecycleId] <- &alertops.SyncRequest{
			LifecycleId: lifecycleId,
			SyncId:      payload.syncId,
			Items: []*alertingv1.PutConfigRequest{
				{
					Key:    payload.configKey,
					Config: payload.data,
				},
			},
		}:
		default:
			s.lg.With("syncer-id", lifecycleId).Error("failed to push sync request : buffer already full")
		}
	}
}

func NewSyncController(lg *zap.SugaredLogger) SyncController {
	return SyncController{
		lg:              lg,
		syncPushers:     map[string]chan *alertops.SyncRequest{},
		remoteInfo:      map[string]RemoteInfo{},
		syncMu:          sync.RWMutex{},
		remoteMu:        sync.RWMutex{},
		hashMu:          sync.Mutex{},
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

func (p *Plugin) Info(ctx context.Context, _ *emptypb.Empty) (*alertops.ComponentInfo, error) {
	remoteInfo := p.syncController.ListRemoteInfo()
	ctxTimeout, ca := context.WithTimeout(ctx, 1*time.Second)
	defer ca()
	cl, err := p.storageClientSet.GetContext(ctxTimeout)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	p.syncController.hashMu.Lock()
	hash, err := cl.GetHash(ctx, shared.SingleConfigId)
	p.syncController.hashMu.Unlock()
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("could not find or calculate hash for %s", shared.SingleConfigId))
	}
	res := &alertops.ComponentInfo{
		CurSyncId:  hash,
		Components: map[string]*alertops.Component{},
	}
	for lifecycleId, info := range remoteInfo {
		res.Components[info.WhoAmI] = &alertops.Component{
			ConnectInfo: &alertops.ConnectInfo{
				LifecycleUuid: lifecycleId,
				Whoami:        info.WhoAmI,
				State:         info.LastSyncState,
				SyncId:        info.LastSyncId,
			},
			LastHandshake: info.LastApplied,
		}
	}

	return res, nil
}

func (p *Plugin) constructManualSync() (*syncPayload, error) {
	ctxTimeout, ca := context.WithTimeout(p.ctx, 5*time.Second)
	defer ca()
	storageClientSet, err := p.storageClientSet.GetContext(ctxTimeout)
	if err != nil {
		return nil, status.Error(codes.Unavailable, fmt.Sprintf("failed to get storage client set: %s", err))
	}
	driver, err := p.clusterDriver.GetContext(ctxTimeout)
	if err != nil {
		return nil, status.Error(codes.Unavailable, fmt.Sprintf("failed to get cluster driver: %s", err))
	}
	return p.constructPartialSyncRequest(p.ctx, driver, storageClientSet, storageClientSet.Routers())
}

func (p *Plugin) SyncConfig(server alertops.ConfigReconciler_SyncConfigServer) error {
	assignedLifecycleUuid := uuid.New().String()
	lg := p.logger.With("method", "SyncConfig", "assignedId", assignedLifecycleUuid)
	lg.Infof(" remote syncer connected, performing initial sync...")
	syncChan := make(chan *alertops.SyncRequest, 16)
	defer close(syncChan)
	p.syncController.AddSyncPusher(assignedLifecycleUuid, syncChan)
	defer p.syncController.RemoveRemoteInfo(assignedLifecycleUuid)
	defer p.syncController.RemoveSyncPusher(assignedLifecycleUuid)

	payload, err := p.constructManualSync()
	if err != nil {
		return err
	}
	if err := server.Send(&alertops.SyncRequest{
		LifecycleId: assignedLifecycleUuid,
		SyncId:      payload.syncId,
		Items: []*alertingv1.PutConfigRequest{
			{
				Key:    payload.configKey,
				Config: payload.data,
			},
		},
	}); err != nil {
		return err
	}
	connErr := lo.Async(func() error {
		for {
			info, err := server.Recv()
			if err == io.EOF {
				lg.Warn("remote syncer closed connection")
				p.syncController.RemoveSyncPusher(assignedLifecycleUuid)
				return nil
			}
			if err != nil {
				return err
			}
			p.syncController.AddRemoteInfo(assignedLifecycleUuid, RemoteInfo{
				WhoAmI:        info.Whoami,
				LastApplied:   timestamppb.Now(),
				LastSyncState: info.State,
				LastSyncId:    info.LifecycleUuid,
			})
		}
	})
	var mu sync.Mutex
	for {
		select {
		case <-p.ctx.Done():
			lg.Info("exiting syncer loop, alerting plugin shutting down")
			return nil
		case <-server.Context().Done():
			lg.Info("exiting syncer loop, remote syncer shutting down")
			return nil
		case err := <-connErr:
			return err
		case syncReq := <-syncChan:
			mu.Lock()
			err := server.Send(syncReq)
			mu.Unlock()
			if err != nil {
				lg.Errorf("could not send sync request to remote syncer %s", err)
			}
		}
	}
}

type syncPayload struct {
	syncId    string
	configKey string
	data      []byte
}

func (p *Plugin) constructPartialSyncRequest(
	ctx context.Context,
	driver drivers.ClusterDriver,
	hashRing spec.HashRing,
	routers spec.RouterStorage,
) (*syncPayload, error) {
	p.syncController.hashMu.Lock()
	defer p.syncController.hashMu.Unlock()

	lg := p.logger.With("method", "constructSyncRequest")
	hash, err := hashRing.GetHash(ctx, shared.SingleConfigId)
	if err != nil {
		lg.Errorf("failed to get hash for %s: %s", shared.SingleConfigId, err)
		panic(err)
	}
	key := shared.SingleConfigId
	router, err := routers.Get(ctx, key)
	if err != nil {
		lg.Errorf("failed to get router %s from storage: %s", key, err)
		return nil, err
	}
	if recv := driver.GetDefaultReceiver(); recv != nil {
		router.SetDefaultReceiver(*recv)
	}
	config, err := router.BuildConfig()
	if err != nil {
		lg.Errorf("failed to build config for router %s: %s", key, err)
		return nil, err
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		lg.Errorf("failed to marshal config for router %s: %s", key, err)
		return nil, err
	}

	return &syncPayload{
		syncId:    hash,
		configKey: key,
		data:      data,
	}, nil
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
	driver, err := p.clusterDriver.GetContext(ctxTimeout)
	if err != nil {
		lg.Warn("failed to acquire cluster driver, skipping sync...")
		return err
	}

	routerKeys, err := clientSet.Sync(ctx)
	if err != nil {
		lg.Errorf("failed to sync configuration in alerting clientset %s", err)
		return err
	}
	if len(routerKeys) > 0 { // global configuration has changed and never been applied

		payload, err := p.constructPartialSyncRequest(ctx, driver, clientSet, clientSet.Routers())
		if err != nil {
			return err
		}
		p.syncController.PushSyncReq(payload)
		return nil
	}
	// check if any sync info is out of date
	remoteSyncInfo := p.syncController.ListRemoteInfo()
	queuedSyncs := []string{}
	for lifecycleId, info := range remoteSyncInfo {
		if info.LastSyncState == alertops.SyncState_SyncUnknown ||
			info.LastSyncState == alertops.SyncState_SyncError {
			queuedSyncs = append(queuedSyncs, lifecycleId)
		}
	}
	if len(queuedSyncs) > 0 {
		payload, err := p.constructPartialSyncRequest(ctx, driver, clientSet, clientSet.Routers())
		if err != nil {
			return err
		}
		for _, id := range queuedSyncs {
			p.syncController.PushOne(id, payload)
		}
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
	driver, err := p.clusterDriver.GetContext(ctxTimeout)
	if err != nil {
		lg.Warn("failed to acquire cluster driver, skipping force sync...")
		return err
	}
	if err := clientSet.ForceSync(ctx); err != nil {
		lg.Errorf("failed to force sync configuration in alerting clientset %s", err)
		return err
	}
	payload, err := p.constructPartialSyncRequest(p.ctx, driver, clientSet, clientSet.Routers())
	if err != nil {
		return err
	}
	p.syncController.PushSyncReq(payload)
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

	var eg util.MultiErrGroup
	for _, task := range tasks {
		task := task
		eg.Go(func() error {
			return task(ctx, syncInfo)
		})
	}
	eg.Wait()
	if err := eg.Error(); err != nil {
		lg.Errorf(" ran %d/%d tasks successfully %s", len(tasks)-len(eg.Errors()), len(tasks), err)
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
