@if(test)
package main

import (
	"list"
	"strings"
	"github.com/rancher/opni/internal/ginkgo"
)

packages: {
	all:         string @tag(packages)
	unit:        string @tag(packages_unit)
	integration: string @tag(packages_integration)
	e2e:         string @tag(packages_e2e)
	temporal:    string @tag(packages_temporal)
	controller:  string @tag(packages_controller)
	slow:        string @tag(packages_slow)
}

pkgTests: {
	_allList:      strings.Split(packages.all, ",")
	_temporalList: strings.Split(packages.temporal, ",")

	all: [ for p in _allList if strings.HasPrefix(p, "./pkg/") {p}]
	nonTemporal: [ for p in all if !list.Contains(_temporalList, p) {p}]
	temporal: [ for p in all if list.Contains(_temporalList, p) {p}]
}

tests: ginkgo.#TestPlan & {
	Parallel: false
	Actions: {
		pkg: ginkgo.#Run & {
			Packages: strings.Join(pkgTests.nonTemporal, ",")
		}
		pkg_temporal: ginkgo.#Run & {
			Packages: strings.Join(pkgTests.temporal, ",")
			Build: {
				Race: false
			}
		}
		controller: ginkgo.#Run & {
			Packages: "./controllers"
			Build: CoverPkg: "github.com/rancher/opni/controllers,github.com/rancher/opni/pkg/resources/.../..."
		}
		integration: ginkgo.#Run & {
			Packages: "./test/functional/...,./test/integration/..."
			Build: CoverPkg: "github.com/rancher/opni/pkg/agent,github.com/rancher/opni/pkg/gateway"
		}
		e2e: ginkgo.#Run & {
			Packages: "./test/e2e/..."
			Explicit: true
		}
	}
	Coverage: {
		MergeProfiles: true
		ExcludePatterns: [
			"**/*.pb.go",
			"**/*.pb*.go",
			"**/zz_*.go",
			"pkg/test",
		]
	}
}
