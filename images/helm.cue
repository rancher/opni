package images

import (
	"universe.dagger.io/docker"
)

#Helm: {
	docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "debian:stable-slim"
			},
			installers.debian.#Helm,
		]
	}
}
