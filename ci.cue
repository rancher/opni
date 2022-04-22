package main

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"universe.dagger.io/docker/cli"
	"universe.dagger.io/alpine"
	"encoding/json"
)

scratch: docker.#Image & {
	rootfs: dagger.#Scratch
	config: {}
}

#Web: {
	repo:           string
	branch:         string
	revision:       string
	buildImage:     string
	pullBuildImage: bool

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
						args: ["generate", "-c", "product/opni/nuxt.config.js"]
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
				input:    scratch
				contents: dist
				dest:     "/app/dist"
			},
		]
	}

	output: cache.output
}

dagger.#Plan & {
	client: {
		env: {
			DRONE_TAG?:          string
			KUBECONFIG?:         string
			TAG:                 string | *"latest"
			REPO:                string | *"rancher"
			OPNI_UI_REPO:        string | *"rancher/opni-ui"
			OPNI_UI_BRANCH:      string | *"main"
			OPNI_UI_BUILD_IMAGE: string | *"kralicky/opni-monitoring-ui-build"
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
			"bin": write: contents:      actions.build.bin
			"web/dist": write: contents: actions.web.dist
		}
		commands: {
			uiVersion: {
				name: "curl"
				args: [
					"-s",
					"https://api.github.com/repos/\(env.OPNI_UI_REPO)/git/refs/heads/\(env.OPNI_UI_BRANCH)",
				]
			}
			buildCacheImageExists: {
				name: "curl"
				args: [
					"-s",
					"https://index.docker.io/v1/repositories/\(env.OPNI_UI_BUILD_IMAGE)/tags/\(actions.web.revision)",
				]
			}
		}
		network: "unix:///var/run/docker.sock": connect: dagger.#Socket
	}

	actions: {
		// - Build web assets
		web: #Web & {
			_githubApiResponse: json.Unmarshal(client.commands.uiVersion.stdout)
			_existsLocal:       cli.#Run & {
				command: {
					name: "image"
					args: [
						"inspect", _taggedImage,
					]
				}
			}

			revision:      _githubApiResponse.object.sha
			_taggedImage:  "\(client.env.OPNI_UI_BUILD_IMAGE):\(revision)"
			_existsLocal:  _existsLocal.stderr == ""
			_existsRemote: client.commands.buildCacheImageExists.stdout == "[]"

			repo:           client.env.OPNI_UI_REPO
			branch:         client.env.OPNI_UI_BRANCH
			buildImage:     _taggedImage
			pullBuildImage: _existsLocal || _existsRemote | *false
		}

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

		// - Build with mage using the builder image
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
					docker.#Copy & {
						contents: actions.web.dist
						dest:     "/src/web/dist/"
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
				"curl":            _
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

		// - Load web asset cache image into the local docker daemon
		webcache: cli.#Load & {
			image: web.output
			host:  client.network."unix:///var/run/docker.sock".connect
			tag:   web.buildImage
		}

		// - Run unit and integration tests
		test: #MageRun & {
			input:       build.output
			mountSource: client.filesystem.".".read.contents
			mageArgs: ["-v", "test"]
			always: true
		}

		// - Run end-to-end tests
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
