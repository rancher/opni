package slo

import (
	"context"

	v1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/hashicorp/go-hclog"
	apis "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/proto"
)

var datasourceToImpl map[string]SLOStore = make(map[string]SLOStore)

func RegisterDatasource(datasource string, impl SLOStore) {
	datasourceToImpl[datasource] = impl
}

type SLOStore interface {
	// This method has to handle storage of the SLO in the KVStore itself
	// since there can be partial successes inside the method
	Create([]v1.SLO) (*apis.CreatedSLOs, error)
	Update(osloSpecs []v1.SLO, existing *sloapi.SLOImplData) (*sloapi.SLOImplData, error)
	Delete(existing *sloapi.SLOImplData) error
	Clone(clone *sloapi.SLOImplData) (string, error)
	Status(existing *sloapi.SLOImplData) (*sloapi.SLOStatus, error)
	WithCurrentRequest(req proto.Message, ctx context.Context) SLOStore
}

type SLORequestBase struct {
	req proto.Message
	p   *Plugin
	ctx context.Context
	lg  hclog.Logger
}

type SLOMonitoring struct {
	SLORequestBase
}

type SLOLogging struct {
	SLORequestBase
}

func NewSLOMonitoringStore(p *Plugin, lg hclog.Logger) SLOStore {
	return SLOMonitoring{
		SLORequestBase{
			req: nil,
			p:   p,
			ctx: context.TODO(),
			lg:  lg,
		},
	}
}

// func (s SLOLogging) NewSLOLoggingStore(req proto.Message, p *Plugin, ctx context.Context, lg hclog.Logger) SLOStore {
// 	return SLOLogging{
// 		SLORequestBase{
// 			req: nil,
// 			p:   p,
// 			ctx: ctx,
// 			lg:  lg,
// 		},
// 	}
// }
