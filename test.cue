@if(test)
package main

import (
	"github.com/rancher/opni/internal/ginkgo"
)

tests: ginkgo.#TestPlan & {
	parallel: true
	actions: {
		unit: ginkgo.#Run & {
			packages: "./pkg/..."
			cover: {
				enabled: true
				coverpkg: ["github.com/rancher/opni/pkg/..."]
			}
		}
		controllers: ginkgo.#Run & {
			packages: "./controllers"
			cover: {
				enabled: true
				coverpkg: [
					"github.com/rancher/opni/controllers",
					"github.com/rancher/opni/pkg/resources/.../..."
				]
			}
		}
	}
	coverage: {
		mergeReports: true
	}
}
