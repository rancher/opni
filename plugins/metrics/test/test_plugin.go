package test

import (
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway"
)

func init() {
	test.EnablePlugin(meta.ModeGateway, gateway.Scheme)
	test.EnablePlugin(meta.ModeAgent, agent.Scheme)
}
