package slo

import (
	"context"

	v1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/hashicorp/go-hclog"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/proto"
)

var datasourceToSLO map[string]SLOStore = make(map[string]SLOStore)
var datasourceToService map[string]ServiceBackend = make(map[string]ServiceBackend)

func RegisterDatasource(datasource string, sloImpl SLOStore, serviceImpl ServiceBackend) {
	datasourceToSLO[datasource] = sloImpl
	datasourceToService[datasource] = serviceImpl
}

type SLOStore interface {
	// This method has to handle storage of the SLO in the KVStore itself
	// since there can be partial successes inside the method
	Create([]v1.SLO) (*corev1.ReferenceList, error)
	Update(osloSpecs []v1.SLO, existing *sloapi.SLOData) (*sloapi.SLOData, error)
	Delete(existing *sloapi.SLOData) error
	Clone(clone *sloapi.SLOData) (string, error)
	Status(existing *sloapi.SLOData) (*sloapi.SLOStatus, error)
	WithCurrentRequest(req proto.Message, ctx context.Context) SLOStore
}
type ServiceBackend interface {
	List(clusters *corev1.ClusterList) (*sloapi.ServiceList, error)
	GetMetricId() (*MetricIds, error)
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
	return SLOMonitoring{
		RequestBase{
			req: nil,
			p:   p,
			ctx: context.TODO(),
			lg:  lg,
		},
	}
}

func NewMonitoringServiceBackend(p *Plugin, lg hclog.Logger) ServiceBackend {
	return MonitoringServiceBackend{
		RequestBase{
			req: nil,
			p:   p,
			ctx: context.TODO(),
			lg:  lg,
		},
	}
}
