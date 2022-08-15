package slo

import (
	"context"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"sync"

	"github.com/hashicorp/go-hclog"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/proto"
)

var datasourceToSLO map[string]SLOStore = make(map[string]SLOStore)
var datasourceToService map[string]ServiceBackend = make(map[string]ServiceBackend)
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
	Create(s *SLO) error
	Update(new *SLO, existing *sloapi.SLOData) (*sloapi.SLOData, error)
	Delete(existing *sloapi.SLOData) error
	Clone(clone *sloapi.SLOData) (*corev1.Reference, *sloapi.SLOData, error)
	Status(existing *sloapi.SLOData) (*sloapi.SLOStatus, error)
	Preview(s *SLO) (*sloapi.SLOPreviewResponse, error)
	WithCurrentRequest(req proto.Message, ctx context.Context) SLOStore
}
type ServiceBackend interface {
	ListServices() (*sloapi.ServiceList, error)
	ListMetrics() (*sloapi.MetricList, error)
	ListEvents() (*sloapi.EventList, error)
	WithCurrentRequest(req proto.Message, ctx context.Context) ServiceBackend
}

type MetricIds struct {
	Good  string
	Total string
}

type RequestBase struct {
	req proto.Message
	p   *Plugin
	ctx context.Context
	lg  hclog.Logger
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

func NewSLOMonitoringStore(p *Plugin, lg hclog.Logger) SLOStore {
	return &SLOMonitoring{
		RequestBase{
			req: nil,
			p:   p,
			ctx: context.Background(),
			lg:  lg,
		},
	}
}

func NewMonitoringServiceBackend(p *Plugin, lg hclog.Logger) ServiceBackend {
	return &MonitoringServiceBackend{
		RequestBase{
			req: nil,
			p:   p,
			ctx: context.TODO(),
			lg:  lg,
		},
	}
}
