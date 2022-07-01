package images

import (
	"universe.dagger.io/docker"
)

#Plugin: {
	kind:     string | *"resource"
	name:     string
	version?: string
}

#Pulumi: {
	plugins: [...#Plugin] | *[]

	docker.#Build & {
		steps: [
			docker.#Pull & {
				source: "kralicky/pulumi:latest"
			},
			for p in plugins {
				docker.#Run & {
					command: {
						name: "plugin"
						args: [
							"install",
							p.kind,
							p.name,
							if p.version != _|_ {
								p.version
							},
						]
					}
				}
			},
		]
	}
}
