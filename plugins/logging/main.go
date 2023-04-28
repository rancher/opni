package main

import (
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/logging/pkg/agent"
	"github.com/rancher/opni/plugins/logging/pkg/gateway"

	_ "github.com/rancher/opni/plugins/logging/pkg/agent/drivers/kubernetes_manager"
	_ "github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend/kubernetes_manager"
	_ "github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/management/kubernetes_manager"
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
