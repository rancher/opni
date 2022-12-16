package main

import (
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/aiops/pkg/gateway"
)

func main() {
	m := plugins.Main{
		Modes: meta.ModeSet{
			meta.ModeGateway: gateway.Scheme,
		},
	}
	m.Exec()
}
