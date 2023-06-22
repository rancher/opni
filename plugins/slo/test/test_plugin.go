package test

import (
	"time"

	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
)

func init() {
	sloapi.MinEvaluateInterval = time.Second
	test.EnablePlugin(meta.ModeGateway, slo.Scheme)
}
