package slo

import (
	"context"

	v1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/hashicorp/go-hclog"
	"github.com/rancher/opni/pkg/slo/shared"
	apis "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/proto"
)

var datasourceToImpl map[string]SLOImpl = make(map[string]SLOImpl)

func init() {
	datasourceToImpl[shared.MonitoringDatasource] = SLOMonitoring{}
}

type SLOImpl interface {
	New(req proto.Message, p *Plugin, ctx context.Context, lg hclog.Logger) SLOImpl
	// This method signature has to handle storage of the SLO in the KVStore
	// Since there can be partial successes
	createSLOImpl([]v1.SLO) (*apis.CreatedSLOs, error)
	updateSLOImpl(osloSpecs []v1.SLO, existing *sloapi.SLOImplData) (*sloapi.SLOImplData, error)
	deleteSLOImpl(existing *sloapi.SLOImplData) error
	cloneSLOImpl(clone *sloapi.SLOImplData) (string, error)
	status(existing *sloapi.SLOImplData) (*sloapi.SLOStatus, error)
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

func (s SLOMonitoring) New(req proto.Message, p *Plugin, ctx context.Context, lg hclog.Logger) SLOImpl {
	return SLOMonitoring{
		SLORequestBase{req: req,
			p:   p,
			ctx: ctx,
			lg:  lg,
		},
	}
}

func (s SLOLogging) New(req proto.Message, p *Plugin, ctx context.Context, lg hclog.Logger) SLOImpl {
	return SLOMonitoring{
		SLORequestBase{req: req,
			p:   p,
			ctx: ctx,
			lg:  lg,
		},
	}
}
