package targets

import (
	// mage:import charts
	charts "github.com/rancher/charts-build-scripts/pkg/actions"

	"github.com/magefile/mage/mg"
)

func Charts() {
	mg.SerialDeps(func() {
		charts.Charts("opni")
	}, func() {
		charts.Charts("opni-agent")
	})
}
