package mage

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

#Run: {
	mageArgs: [...string]
	input: docker.#Image
	env: [string]: string | dagger.#Secret
	_defaultEnv: {
		GOMODCACHE: "/root/.cache/go-mod"
	}

	docker.#Run & {
		workdir: "/src"
		mounts: {
			"go mod cache": {
				contents: core.#CacheDir & {
					id: "go_mod"
				}
				dest: "/root/.cache/go-mod"
			}
			"go build cache": {
				contents: core.#CacheDir & {
					id: "go_build"
				}
				dest: "/root/.cache/go-build"
			}
		}
		"env": env & _defaultEnv
		command: {
			name: "mage"
			args: mageArgs
		}
	}
}

#Image: docker.#Build & {
	steps: [
		docker.#Pull & {
			source: "golang:1.18"
		},
		docker.#Run & {
			command: {
				name: "go"
				args: ["install", "github.com/magefile/mage@latest"]
			}
		},
	]
}
