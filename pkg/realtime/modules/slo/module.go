package slo

import (
	"context"
	"fmt"

	"github.com/iancoleman/strcase"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/realtime/modules"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

type module struct {
	mc                *modules.ModuleContext
	sloClient         slo.SLOClient
	cortexAdminClient cortexadmin.CortexAdminClient
	events            chan *sloEvent
}

func AddToModuleSet(set *modules.ModuleSet) {
	set.Add("slo", &module{})
}

func (m *module) Run(ctx context.Context, mc *modules.ModuleContext) error {
	m.mc = mc
	lg := mc.Log
	lg.Debug("initializing gateway extension clients")
	m.events = make(chan *sloEvent, 1000)

	var err error
	m.cortexAdminClient, err = clients.FromExtension(
		ctx, m.mc.Client, "CortexAdmin", cortexadmin.NewCortexAdminClient)
	if err != nil {
		return fmt.Errorf("failed to initialize cortex admin client: %w", err)
	}

	m.sloClient, err = clients.FromExtension(
		ctx, m.mc.Client, "SLO", slo.NewSLOClient)
	if err != nil {
		return fmt.Errorf("failed to initialize slo client: %w", err)
	}

	lg.Debug("initialized gateway extension clients successfully")

	go m.manageTasks(ctx, func(slo *slo.SLOData) task {
		return &monitor{
			slo:               slo,
			mgmtClient:        m.mc.Client,
			cortexAdminClient: m.cortexAdminClient,
			sloClient:         m.sloClient,
			logger:            lg.Named(strcase.ToKebab(slo.SLO.GetName())),
		}
	})
	go m.watchEvents(ctx)
	<-ctx.Done()
	return nil
}
