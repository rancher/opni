package main

import (
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/aiops/pkg/gateway"

	_ "github.com/rancher/opni/plugins/aiops/pkg/features/log_anomaly"
	_ "github.com/rancher/opni/plugins/aiops/pkg/features/metric_anomaly"
	_ "github.com/rancher/opni/plugins/aiops/pkg/features/metric_anomaly/drivers/opni_manager"
)

func main() {
	m := plugins.Main{
		Modes: meta.ModeSet{
			meta.ModeGateway: gateway.Scheme,
		},
	}
	m.Exec()
}
