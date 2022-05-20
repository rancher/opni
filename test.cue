@if(test)
package main

import (
	"github.com/rancher/opni/internal/ginkgo"
	"strings"
)

testPackages: string @tag(packages)

tests: ginkgo.#TestPlan & {
	Parallel: false
	Actions: {
		pkg: ginkgo.#Run & {
			_pkgs: strings.Split(testPackages, ",")
			_filtered: [ for p in _pkgs if strings.HasPrefix(p, "./pkg/") {p}]
			Packages: strings.Join(_filtered, ",")
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
