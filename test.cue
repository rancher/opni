@if(test)
package main

import (
	"list"
	"strings"
	"pkg.go.dev/time"
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
	deprecated:  string @tag(packages_deprecated)
	aberrant:    string @tag(packages_aberrant)
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
			Suite: LabelFilter: "temporal"
			Build: {
				Race:  false
				Cover: false
			}
		}
		controller: ginkgo.#Run & {
			Packages: "./controllers"
			Build: CoverPkg:    "github.com/rancher/opni/controllers,github.com/rancher/opni/pkg/resources/.../..."
			Suite: LabelFilter: "!deprecated"
		}
		logging: ginkgo.#Run & {
			Packages: "./plugins/logging/pkg/gateway"
			Build: CoverPkg: "github.com/rancher/opni/plugins/logging/pkg/gateway"
		}
		plugins: ginkgo.#Run & {
			Packages: "./test/plugins/..."
			Build: CoverPkg: "github.com/rancher/opni/plugins/..."
			Run: Parallel:   true
		}
		integration: ginkgo.#Run & {
			Packages: "./test/integration/..."
			Build: CoverPkg:    "github.com/rancher/opni/pkg/agent/v2,github.com/rancher/opni/pkg/gateway"
			Suite: LabelFilter: "!aberrant"
			Run: Parallel:      true
		}
		e2e: ginkgo.#Run & {
			Packages: "./test/e2e/..."
			Explicit: true
			Suite: {
				Timeout: 1 * time.#Hour
			}
		}
		aberrant: ginkgo.#Run & {
			Packages: packages.aberrant
			Explicit: true
			Build: {
				Race:  false
				Cover: false
			}
			Suite: LabelFilter: "aberrant"
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
