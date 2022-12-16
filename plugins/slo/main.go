package main

import (
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
)

func main() {
	m := plugins.Main{
		Modes: meta.ModeSet{
			meta.ModeGateway: slo.Scheme,
		},
	}
	m.Exec()
}
