@if(!test)
package main

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"universe.dagger.io/docker/cli"
	"universe.dagger.io/alpine"
	"github.com/rancher/opni/internal/builders"
	"github.com/rancher/opni/internal/mage"
	"github.com/rancher/opni/internal/util"
	"universe.dagger.io/x/ezequiel@foncubierta.com/terraform"
)

dagger.#Plan & {
	client: {
		env: {
			GINKGO_LABEL_FILTER: string | *""
			DRONE:               string | *""
			KUBECONFIG:          string | *""
			TAG:                 string | *"latest"
			REPO:                string | *"rancher"
			IMAGE_NAME:          string | *"opni"
			OPNI_UI_REPO:        string | *"rancher/opni-ui"
			OPNI_UI_BRANCH:      string | *"main"
			OPNI_UI_BUILD_IMAGE: string | *"rancher/opni-monitoring-ui-build"
			DASHBOARDS_VERSION:  string | *"1.3.1"
			OPENSEARCH_VERSION:  string | *"1.3.1"
			PLUGIN_VERSION:      string | *"0.5.3"
			PLUGIN_PUBLISH:      string | *"0.5.4-rc3"
			DOCKER_USERNAME?:    string
			DOCKER_PASSWORD?:    dagger.#Secret
			EXPECTED_REF?:       string // used by tilt
		}
		filesystem: {
			".": read: {
				contents: dagger.#FS
				exclude: [
					"bin",
					"dev",
					"docs",
					"ci.cue",
					"testbin",
					"test/tf/",
					"internal/cmd/testenv",
				]
			}
			"bin": write: contents:       actions.build.bin
			"web/dist": write: contents:  actions.web.dist
			"cover.out": write: contents: actions.test.export.files["/src/cover.out"]

			"dev/terraform.tfvars.json": read: contents: dagger.#Secret
			"test/tf/providers.tf": read: contents:      string
			"test/tf/": {
				read: {
					contents: dagger.#FS
					exclude: [
						".terraform",
						".terraform.lock.hcl",
					]
				}
				write: contents: actions.e2eapply.output
			}
		}
		network: "unix:///var/run/docker.sock": connect: dagger.#Socket
	}

	actions: {
		_uiVersion: util.#FetchJSON & {
			source: "https://api.github.com/repos/\(client.env.OPNI_UI_REPO)/git/refs/heads/\(client.env.OPNI_UI_BRANCH)"
		}
		_buildCacheImageExists: util.#FetchJSON & {
			source: "https://index.docker.io/v1/repositories/\(client.env.OPNI_UI_BUILD_IMAGE)/tags/\(actions.web.revision)"
		}

		// Build web assets
		web: builders.#Web & {
			revision:       _uiVersion.output.object.sha
			buildImage:     "\(client.env.OPNI_UI_BUILD_IMAGE):\(revision)"
			repo:           client.env.OPNI_UI_REPO
			branch:         client.env.OPNI_UI_BRANCH
			pullBuildImage: _buildCacheImageExists.output == [] | *false
		}

		_mageImage: mage.#Image

		// Build with mage using the builder image
		build: {
			docker.#Build & {
				steps: [
					docker.#Copy & {
						input:    _mageImage.output
						contents: client.filesystem.".".read.contents
						dest:     "/src"
					},
					docker.#Copy & {
						contents: client.filesystem.".".read.contents
						source:   "config/assets/nfd"
						dest:     "/opt/nfd/"
					},
					docker.#Copy & {
						contents: client.filesystem.".".read.contents
						source:   "config/assets/gpu-operator"
						dest:     "/opt/gpu-operator/"
					},
					docker.#Copy & {
						contents: actions.web.dist
						dest:     "/src/web/dist/"
					},
					mage.#Run & {
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
					dest:     "/var/lib/opni/plugins/"
					exclude: ["plugin_example"]
				},
				docker.#Copy & {
					contents: build.opt
					dest:     "/opt/"
				},
				docker.#Set & {
					config: {
						entrypoint: ["/usr/bin/opni"]
						env: {
							NVIDIA_VISIBLE_DEVICES: "void"
						}
					}
				},
			]
		}

		_opniImage: {
			tag: docker.#Ref
			if client.env.EXPECTED_REF == _|_ {
				tag: docker.#Ref & "\(client.env.REPO)/\(client.env.IMAGE_NAME):\(client.env.TAG)"
			}
			if client.env.EXPECTED_REF != _|_ {
				tag: docker.#Ref & client.env.EXPECTED_REF
			}
			image: _multistage.output
		}

		// Build docker images and load them into the local docker daemon
		load: {
			opni: cli.#Load & _opniImage & {
				host: client.network."unix:///var/run/docker.sock".connect
			}

			webcache: cli.#Load & {
				image: web.output
				host:  client.network."unix:///var/run/docker.sock".connect
				tag:   web.buildImage
			}
			aiops: cli.#Load & {
				image: actions.aiops.build.output
				host:  client.network."unix:///var/run/docker.sock".connect
				tag:   "\(client.env.REPO)/opni-aiops:\(client.env.TAG)"
			}
		}

		// Run unit and integration tests
		test: mage.#Run & {
			input: build.output
			mageArgs: ["-v", "test"]
			always: true
			env: {
				if client.env.DRONE != "" {
					"DRONE": client.env.DRONE
				}
				if client.env.GINKGO_LABEL_FILTER != "" {
					"GINKGO_LABEL_FILTER": client.env.GINKGO_LABEL_FILTER
				}
			}
			export: files: "/src/cover.out": string
		}

		// Run end-to-end tests

		_e2evars: {
			env: [string]: string | dagger.#Secret
			_vars: client.filesystem."dev/terraform.tfvars.json".read.contents
			if _vars != _|_ {
				_decoded: core.#DecodeSecret & {
					input:  _vars
					format: "json"
				}
				env: {
					for key, value in _decoded.output {
						"TF_VAR_\(key)": value.contents
					}
				}
			}
		}

		e2einit: terraform.#Init & {
			_src: core.#WriteFile & {
				input:    dagger.#Scratch
				path:     "providers.tf"
				contents: client.filesystem."test/tf/providers.tf".read.contents
			}
			source: _src.output
		}

		e2eapply: terraform.#Run & _e2evars & {
			always: true
			cmd:    "apply"
			cmdArgs: ["-auto-approve"]
			_src: core.#Merge & {
				inputs: [
					e2einit.output,
					client.filesystem."test/tf/".read.contents,
				]
			}
			source: _src.output
		}

		// Push docker images
		push: {
			opni: docker.#Push & {
				dest:  _opniImage.tag
				image: _opniImage.image
				if client.env.DOCKER_USERNAME != _|_ && client.env.DOCKER_PASSWORD != _|_ {
					auth: {
						username: client.env.DOCKER_USERNAME
						secret:   client.env.DOCKER_PASSWORD
					}
				}
			}
			webcache: docker.#Push & {
				dest:  web.buildImage
				image: web.output
				if client.env.DOCKER_USERNAME != _|_ && client.env.DOCKER_PASSWORD != _|_ {
					auth: {
						username: client.env.DOCKER_USERNAME
						secret:   client.env.DOCKER_PASSWORD
					}
				}
			}
		}

		dashboards: {
			build: docker.#Build & {
				steps: [
					docker.#Pull & {
						source: "opensearchproject/opensearch-dashboards:\(client.env.DASHBOARDS_VERSION)"
					},
					docker.#Run & {
						command: {
							name: "opensearch-dashboards-plugin"
							args: [
								"install",
								"https://github.com/rancher/opni-ui/releases/download/plugin-\(client.env.PLUGIN_VERSION)/opni-dashboards-plugin-\(client.env.PLUGIN_VERSION).zip",
							]
						}
					},
				]
			}
			push: docker.#Push & {
				dest:  "\(client.env.REPO)/opensearch-dashboards:\(client.env.DASHBOARDS_VERSION)-\(client.env.PLUGIN_PUBLISH)"
				image: dashboards.build.output
				if client.env.DOCKER_USERNAME != _|_ && client.env.DOCKER_PASSWORD != _|_ {
					auth: {
						username: client.env.DOCKER_USERNAME
						secret:   client.env.DOCKER_PASSWORD
					}
				}
			}
		}
		opensearch: {
			build: docker.#Build & {
				steps: [
					docker.#Pull & {
						source: "opensearchproject/opensearch:\(client.env.DASHBOARDS_VERSION)"
					},
					docker.#Run & {
						command: {
							name: "opensearch-plugin"
							args: [
								"-s",
								"install",
								"-b",
								"https://github.com/tybalex/opni-preprocessing-plugin/releases/download/\(client.env.PLUGIN_VERSION)/opnipreprocessing.zip",
							]
						}
					},
				]
			}
			push: docker.#Push & {
				dest:  "\(client.env.REPO)/opensearch:\(client.env.OPENSEARCH_VERSION)-\(client.env.PLUGIN_PUBLISH)"
				image: opensearch.build.output
				if client.env.DOCKER_USERNAME != _|_ && client.env.DOCKER_PASSWORD != _|_ {
					auth: {
						username: client.env.DOCKER_USERNAME
						secret:   client.env.DOCKER_PASSWORD
					}
				}
			}
		}
		aiops: {
			build: docker.#Build & {
				steps: [
					docker.#Pull & {
						source: "rancher/opni-python-base:3.8"
					},
					docker.#Copy & {
						contents: client.filesystem.".".read.contents
						source: "aiops/"
						dest: "."
					},
					docker.#Set & {
						config: {
							cmd: ["python", "opensearch-update-service/main.py"]
						}
					}	
				]
			}
		}
	}
}
