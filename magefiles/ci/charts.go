package main

import (
	// mage:import charts
	charts "github.com/rancher/charts-build-scripts/pkg/actions"

	// mage:import targets
	"github.com/rancher/opni/magefiles/targets"

	"github.com/magefile/mage/mg"
)

func Charts() {
	mg.SerialDeps(
		targets.CRD.All,
		func() {
			charts.Charts("opni")
		}, func() {
			charts.Charts("opni-agent")
		})
}
