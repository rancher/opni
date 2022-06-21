package test

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/rancher/opni/pkg/test/testutil"
)

const (
	Unit        = "unit"
	Integration = "integration"
	Controller  = "controller"
	E2E         = "e2e"
	Slow        = "slow"
	Temporal    = "temporal"
)

func EnableIfCI[T any](decorator T) any {
	return testutil.IfCI[any](decorator).Else(ginkgo.Labels{})
}
