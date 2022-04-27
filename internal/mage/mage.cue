package mage

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

#Run: {
	mountSource?: dagger.#FS
	mageArgs: [...string]
	input: docker.#Image
	env: [string]: string | dagger.#Secret
	_defaultEnv: {
		GOMODCACHE: "/root/.cache/go-mod"
		GOOS:       "linux"
		GOARCH:     "amd64"
	}

	docker.#Run & {
		workdir: "/src"
		mounts: {
			if mountSource != _|_ {
				"source": {
					contents: mountSource
					dest:     "/src"
				}
			}
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
		env: env & _defaultEnv
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