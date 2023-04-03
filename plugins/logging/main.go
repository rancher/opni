package main

import (
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/logging/pkg/agent"
	"github.com/rancher/opni/plugins/logging/pkg/gateway"
	"github.com/rancher/opni/plugins/logging/pkg/util"
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

func init() {
	util.SetMockCertReader(&test.TestCertManager{})
}
