package main

import (
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway"

	_ "github.com/rancher/opni/plugins/metrics/pkg/agent/drivers/opni_manager_otel"
	_ "github.com/rancher/opni/plugins/metrics/pkg/agent/drivers/prometheus_operator"
	_ "github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers/opni_manager"
)

func main() {
	m := plugins.Main{
		Modes: meta.ModeSet{
			meta.ModeGateway: gateway.Scheme,
			meta.ModeAgent:   agent.Scheme,
		},
	}
	m.Exec()
}
