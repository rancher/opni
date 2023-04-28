package test

import (
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
)

func init() {
	test.EnablePlugin(meta.ModeGateway, slo.Scheme)
}
