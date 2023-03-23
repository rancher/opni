package test

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/rancher/opni/pkg/test/testutil"
)

// const (
// 	Unit        = "unit"
// 	Integration = "integration"
// 	Controller  = "controller"
// 	E2E         = "e2e"
// 	Slow        = "slow"
// 	Temporal    = "temporal"
// )

func EnableIfCI[T any](decorator T) any {
	return testutil.IfCI[any](decorator).Else(ginkgo.Labels{})
}

func IfLabelFilterMatches(labels ginkgo.Labels, f func()) {
	if labels.MatchesLabelFilter(ginkgo.GinkgoLabelFilter()) {
		fmt.Printf("%s matches label filter %s, running test\n", labels, ginkgo.GinkgoLabelFilter())
		f()
	}
}

func IfIntegration(f func()) {
	IfLabelFilterMatches(ginkgo.Label("integration"), f)
}
