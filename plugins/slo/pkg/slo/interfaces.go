package slo

import (
	"context"
	"sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	sloapi "github.com/rancher/opni/plugins/slo/apis/slo"
	"log/slog"

	"google.golang.org/protobuf/proto"
)

var datasourceToSLO = make(map[string]SLOStore)
var datasourceToService = make(map[string]ServiceBackend)
var mu sync.Mutex

func RegisterDatasource(datasource string, sloImpl SLOStore, serviceImpl ServiceBackend) {
	defer mu.Unlock()
	mu.Lock()
	datasourceToSLO[datasource] = sloImpl
	datasourceToService[datasource] = serviceImpl
}

type SLOStore interface {
	// This method has to handle storage of the SLO in the KVStore itself
	// since there can be partial successes inside the method
	Create() (*corev1.Reference, error)
	Update(existing *sloapi.SLOData) (*sloapi.SLOData, error)
	Delete(existing *sloapi.SLOData) error
	Clone(clone *sloapi.SLOData) (*corev1.Reference, *sloapi.SLOData, error)
	MultiClusterClone(
		base *sloapi.SLOData,
		clusters []*corev1.Reference,
		svcBackend ServiceBackend,
	) ([]*corev1.Reference, []*sloapi.SLOData, []error)
	Status(existing *sloapi.SLOData) (*sloapi.SLOStatus, error)
	Preview(s *SLO) (*sloapi.SLOPreviewResponse, error)
	WithCurrentRequest(ctx context.Context, req proto.Message) SLOStore
}
type ServiceBackend interface {
	ListServices() (*sloapi.ServiceList, error)
	ListMetrics() (*sloapi.MetricGroupList, error)
	ListEvents() (*sloapi.EventList, error)
	WithCurrentRequest(ctx context.Context, req proto.Message) ServiceBackend
}

type MetricIds struct {
	Good  string
	Total string
}

type RequestBase struct {
	req proto.Message
	p   *Plugin
	ctx context.Context
	lg  *slog.Logger
}

type SLOMonitoring struct {
	RequestBase
}

type SLOLogging struct {
	RequestBase
}

type MonitoringServiceBackend struct {
	RequestBase
}

func NewSLOMonitoringStore(p *Plugin, lg *slog.Logger) SLOStore {
	return &SLOMonitoring{
		RequestBase{
			req: nil,
			p:   p,
			ctx: context.Background(),
			lg:  lg,
		},
	}
}

func NewMonitoringServiceBackend(p *Plugin, lg *slog.Logger) ServiceBackend {
	return &MonitoringServiceBackend{
		RequestBase{
			req: nil,
			p:   p,
			ctx: context.TODO(),
			lg:  lg,
		},
	}
}
