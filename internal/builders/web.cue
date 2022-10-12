package builders

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"github.com/rancher/opni/internal/util"
)

#Web: {
	repo:           string
	branch:         string
	revision:       string
	buildImage:     string
	pullBuildImage: bool
	keepSourceMaps: bool | *false

	build: _
	if pullBuildImage {
		build: docker.#Build & {
			steps: [
				docker.#Pull & {
					source: buildImage
				},
			]
		}
	}
	if !pullBuildImage {
		build: docker.#Build & {
			steps: [
				docker.#Pull & {
					source: "node:14-alpine3.10"
				},
				docker.#Run & {
					command: {
						name: "apk"
						args: ["add", "--no-cache", "git", "brotli"]
					}
				},
				docker.#Run & {
					command: {
						name: "echo"
						args: [revision]
					}
				},
				docker.#Run & {
					workdir: "/app"
					command: {
						name: "git"
						args: [
							"clone",
							"--depth=1",
							"--branch=\(branch)",
							"https://github.com/\(repo).git",
							".",
						]
					}
				},
				docker.#Run & {
					workdir: "/app"
					command: {
						name: "git"
						args: [
							"checkout",
							revision,
						]
					}
				},
				docker.#Run & {
					workdir: "/app"
					command: {
						name: "yarn"
						args: ["install"]
					}
				},
				docker.#Run & {
					workdir: "/app"
					command: {
						name: "node_modules/.bin/nuxt"
						args: ["generate", "-c", "product/opni/nuxt.config.js", "--spa"]
					}
				},
				docker.#Run & {
					if !keepSourceMaps {
						command: {
							name: "find"
							args: ["/app/dist", "-type", "f", "-name", "*.map", "-delete"]
						}
					}
					if keepSourceMaps {
						command: name: "/bin/true"
					}
				},
				docker.#Run & {
					workdir: "/app"
					command: {
						name: "find"
						args: ["/app/dist", "-type", "f", "-exec", "brotli", "-jf", "{}", "+"]
					}
				},
			]
		}
	}

	dist:        dagger.#FS & _distSubdir.output
	_distSubdir: core.#Subdir & {
		input: build.output.rootfs
		path:  "/app/dist"
	}

	cache: docker.#Build & {
		steps: [
			docker.#Copy & {
				input:    util.Scratch
				contents: dist
				dest:     "/app/dist"
			},
		]
	}

	output: cache.output
}
