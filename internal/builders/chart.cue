package builders

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

#Chart: {
	crds: [...dagger.#FS]
	chartDir: dagger.#FS

	_merged: core.#Merge & {
		inputs: crds
	}

	build: docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "dtzar/helm-kubectl"
			},
			docker.#Copy & {
				contents: chartDir
				dest:     "/src"
			},
			docker.#Copy & {
				contents: _merged.output
				dest:     "/src/crds"
			},
			docker.#Run & {
				workdir: "/src"
				command: {
					name: "helm"
					args: ["dep", "update"]
				}
			},
			docker.#Run & {
				workdir: "/src"
				command: {
					name: "helm"
					args: ["package", "-d", "/dist", "."]
				}
			},
		]
	}

	_dist: core.#Subdir & {
		input: build.output.rootfs
		path:  "/dist"
	}
	output: _dist.output
}
