package test

import (
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/example/pkg/example"
)

func init() {
	test.EnablePlugin(meta.ModeGateway, example.Scheme)
}
