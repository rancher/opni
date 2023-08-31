package slo

import (
	"context"
	"sync"

	"github.com/rancher/opni/pkg/slo/backend"
	"github.com/rancher/opni/pkg/storage"

	"github.com/rancher/opni/pkg/alerting/server"
	alertingSync "github.com/rancher/opni/pkg/alerting/server/sync"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"go.uber.org/zap"
)

type SLOServerComponent struct {
	slov1.UnsafeSLOServer
	util.Initializer
	logger     *zap.SugaredLogger
	storage    future.Future[StorageAPIs]
	mgmtClient future.Future[managementv1.ManagementClient]
}

var _ server.ServerComponent = (*SLOServerComponent)(nil)
var _ slov1.SLOServer = (*SLOServerComponent)(nil)

type StorageAPIs struct {
	SLOs     storage.KeyValueStoreT[*slov1.SLOData]
	Services storage.KeyValueStoreT[*slov1.Service]
	Metrics  storage.KeyValueStoreT[*slov1.Metric]
}

func NewSLOServerComponent(
	logger *zap.SugaredLogger,
) *SLOServerComponent {
	return &SLOServerComponent{
		logger:     logger,
		storage:    future.New[StorageAPIs](),
		mgmtClient: future.New[managementv1.ManagementClient](),
	}
}

type SLOServerConfiguration struct {
	Storage    StorageAPIs
	MgmtClient managementv1.ManagementClient
}

func (s *SLOServerComponent) Name() string {
	return "notification"
}

func (s *SLOServerComponent) Status() server.Status {
	return server.Status{
		Running: s.Initialized(),
	}
}

func (s *SLOServerComponent) Ready() bool {
	return s.Initialized()
}

func (s *SLOServerComponent) Healthy() bool {
	return s.Initialized()
}

func (s *SLOServerComponent) SetConfig(_ server.Config) {
}

func (s *SLOServerComponent) Sync(_ context.Context, _ alertingSync.SyncInfo) error {
	return nil
}

func (s *SLOServerComponent) Initialize(conf SLOServerConfiguration) {
	s.InitOnce(func() {
		s.mgmtClient.Set(conf.MgmtClient)
		s.storage.Set(conf.Storage)
	})
}

var datasourceMu sync.RWMutex

var (
	datasources = make(map[string]backend.SLODatasource)
)

func RegisterDatasource(
	datasourceName string,
	datasourceImpl backend.SLODatasource,
) {
	defer datasourceMu.Unlock()
	datasourceMu.Lock()
	datasources[datasourceName] = datasourceImpl
}
