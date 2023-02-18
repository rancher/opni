package mage

import (
	"dagger.io/dagger"
	// "dagger.io/dagger/core"
	"universe.dagger.io/docker"
)

#Run: {
	mageArgs: [...string]
	input: docker.#Image
	env: [string]: string | dagger.#Secret

	docker.#Run & {
		workdir: "/src"
		"env":   env
		command: {
			name: "mage"
			args: mageArgs
		}
	}
}

#Image: docker.#Build & {
	steps: [
		docker.#Pull & {
			source: "golang:1.20"
		},
		docker.#Run & {
			command: {
				name: "apt"
				args: ["update"]
			}
		},
		docker.#Run & {
			command: {
				name: "apt"
				args: ["install", "-y", "upx"]
			}
		},
		docker.#Run & {
			command: {
				name: "go"
				args: ["install", "github.com/magefile/mage@latest"]
			}
		},
	]
}
