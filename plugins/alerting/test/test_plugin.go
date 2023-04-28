package test

import (
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

func init() {
	test.EnablePlugin(meta.ModeGateway, alerting.Scheme)
}
