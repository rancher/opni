package testruntime

import (
	"github.com/onsi/ginkgo/v2"
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
	return IfCI[any](decorator).Else(ginkgo.Labels{})
}

func IfLabelFilterMatches(labels ginkgo.Labels, f func()) {
	if labels.MatchesLabelFilter(ginkgo.GinkgoLabelFilter()) {
		f()
	}
}

func IfIntegration(f func()) {
	IfLabelFilterMatches(ginkgo.Label("integration"), f)
}
