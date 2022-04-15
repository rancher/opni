package main

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"universe.dagger.io/docker/cli"
	"universe.dagger.io/alpine"
)

dagger.#Plan & {
	client: {
		env: {
			DRONE_TAG?:  string
			KUBECONFIG?: string
			TAG:         string | *"latest"
			REPO:        string | *"rancher"
		}
		filesystem: {
			".": read: {
				contents: dagger.#FS
				exclude: [
					"bin",
					"dev",
					"docs",
					"build.cue",
					"testbin",
					"internal/cmd/testenv",
				]
			}
			bin: write: contents: actions.build.bin
		}
		network: "unix:///var/run/docker.sock": connect: dagger.#Socket
	}

	actions: {
		// Build golang image with mage installed
		_builder: docker.#Build & {
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
		// Build with mage using the builder image
		build: {
			docker.#Build & {
				steps: [
					docker.#Copy & {
						input:    _builder.output
						contents: client.filesystem.".".read.contents
						dest:     "/src"
					},
					docker.#Copy & {
						contents: client.filesystem.".".read.contents
						source:   "package/assets/nfd"
						dest:     "/opt/nfd/"
					},
					docker.#Copy & {
						contents: client.filesystem.".".read.contents
						source:   "package/assets/gpu-operator"
						dest:     "/opt/gpu-operator/"
					},
					#MageRun & {
						mageArgs: ["-v", "build"]
					},
				]
			}
			bin:        dagger.#FS & _binSubdir.output
			_binSubdir: core.#Subdir & {
				input: actions.build.output.rootfs
				path:  "/src/bin"
			}
			plugins:        dagger.#FS & _pluginsSubdir.output
			_pluginsSubdir: core.#Subdir & {
				input: actions.build.output.rootfs
				path:  "/src/bin/plugins"
			}
			opt:        dagger.#FS & _optSubdir.output
			_optSubdir: core.#Subdir & {
				input: actions.build.output.rootfs
				path:  "/opt"
			}
		}
		// Build the destination base image
		_baseimage: alpine.#Build & {
			packages: {
				"ca-certificates": _
			}
		}
		// Copy the build output to the destination image
		_multistage: docker.#Build & {
			steps: [
				docker.#Copy & {
					input:    _baseimage.output
					contents: build.bin
					source:   "opni"
					dest:     "/usr/bin/opni"
				},
				docker.#Copy & {
					// input connects to previous step's output
					contents: build.plugins
					dest:     "/var/lib/opnim/plugins/"
					exclude: ["plugin_example"]
				},
				docker.#Copy & {
					contents: build.opt
					dest:     "/opt/"
				},
				docker.#Set & {
					config: {
						entrypoint: ["/usr/bin/opnim"]
						env: {
							NVIDIA_VISIBLE_DEVICES: "void"
						}
					}
				},
			]
		}

		// - Build docker image and load it into the local docker daemon
		package: cli.#Load & {
			_tag:  client.env.DRONE_TAG | *client.env.TAG
			image: _multistage.output
			host:  client.network."unix:///var/run/docker.sock".connect
			tag:   "\(client.env.REPO)/opni:\(_tag)"
		}

		// - Run tests
		test: #MageRun & {
			input:       build.output
			mountSource: client.filesystem.".".read.contents
			mageArgs: ["-v", "test"]
			always: true
		}
		e2e: #MageRun & {
			input:       build.output
			mountSource: client.filesystem.".".read.contents
			mageArgs: ["-v", "e2e"]
			always: true
			env: {
				if client.env.KUBECONFIG != "" {
					"KUBECONFIG": client.env.KUBECONFIG
				}
			}
		}
	}
}

#MageRun: {
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
