@if(!test)
package main

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"universe.dagger.io/docker/cli"
	"universe.dagger.io/alpine"
	"universe.dagger.io/git"
	"universe.dagger.io/python"
	"github.com/rancher/opni/internal/builders"
	"github.com/rancher/opni/internal/mage"
	"github.com/rancher/opni/internal/util"
)

dagger.#Plan & {
	client: {
		env: {
			GINKGO_LABEL_FILTER: string | *""
			DRONE:               string | *""
			KUBECONFIG:          string | *""
			TAG:                 string | *"latest"
			REPO:                string | *"rancher"
			OPNI_UI_REPO:        string | *"rancher/opni-ui"
			OPNI_UI_BRANCH:      string | *"main"
			OPNI_UI_BUILD_IMAGE: string | *"rancher/opni-monitoring-ui-build"
			DASHBOARDS_VERSION:  string | *"1.3.1"
			OPENSEARCH_VERSION:  string | *"1.3.1"
			PLUGIN_VERSION:      string | *"0.5.3"
			PLUGIN_PUBLISH:      string | *"0.5.4-rc3"
			DOCKER_USERNAME?:    string
			DOCKER_PASSWORD?:    dagger.#Secret
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
			"bin": write: contents:             actions.build.bin
			"web/dist": write: contents:        actions.web.dist
			"dist/charts": write: contents:     actions.charts.output
			"cover.out": write: contents:       actions.test.export.files["/src/cover.out"]
			"aiops/apis/dist": write: contents: actions.aiops.packages.output
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
						mageArgs: ["-v"]
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
			tag:   docker.#Ref & "\(client.env.REPO)/opni:\(client.env.TAG)"
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
		e2e: mage.#Run & {
			input: build.output
			mageArgs: ["-v", "e2e"]
			always: true
			env: {
				if client.env.KUBECONFIG != "" {
					"KUBECONFIG": client.env.KUBECONFIG
				}
				if client.env.GINKGO_LABEL_FILTER != "" {
					"GINKGO_LABEL_FILTER": client.env.GINKGO_LABEL_FILTER
				}
			}
		}

		// Build and package helm charts (writes to dist/charts/)
		charts: {
			_bases: core.#Subdir & {
				input: client.filesystem.".".read.contents
				path:  "/config/crd/bases"
			}
			_grafana: core.#Subdir & {
				input: client.filesystem.".".read.contents
				path:  "/config/crd/grafana"
			}
			_logging: core.#Subdir & {
				input: client.filesystem.".".read.contents
				path:  "/config/crd/logging"
			}
			_nfd: core.#Subdir & {
				input: client.filesystem.".".read.contents
				path:  "/config/crd/nfd"
			}
			_nvidia: core.#Subdir & {
				input: client.filesystem.".".read.contents
				path:  "/config/crd/nvidia"
			}
			_opensearch: core.#Subdir & {
				input: client.filesystem.".".read.contents
				path:  "/config/crd/opensearch"
			}

			_promOperatorRepo: git.#Pull & {
				remote: "https://github.com/prometheus-community/helm-charts.git"
				ref:    "main"
			}
			_promOperatorCrds: core.#Subdir & {
				input: _promOperatorRepo.output
				path:  "charts/kube-prometheus-stack/crds"
			}

			_opniCrds: [
				_bases.output,
				_grafana.output,
				_logging.output,
				_nfd.output,
				_nvidia.output,
				_opensearch.output,
			]

			agent: builders.#Chart & {
				_chartDir: core.#Subdir & {
					input: client.filesystem.".".read.contents
					path:  "deploy/charts/opni-monitoring-agent"
				}

				chartDir: _chartDir.output
				crds:     _opniCrds
			}

			opni: builders.#Chart & {
				_chartDir: core.#Subdir & {
					input: client.filesystem.".".read.contents
					path:  "deploy/charts/opni"
				}

				chartDir: _chartDir.output
				crds:     _opniCrds + [_promOperatorCrds.output]
			}

			_output: core.#Merge & {
				inputs: [
					charts.agent.output,
					charts.opni.output,
				]
			}
			output: _output.output
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
			packages: {
				sdist: python.#Run & {
					script: {
						directory: actions.build.output.rootfs
						filename:  "src/aiops/apis/setup.py"
					}
					workdir: "/run/python/src/aiops/apis"
					args: ["sdist", "-d", "/dist"]
				}
				_subdir: core.#Subdir & {
					input: sdist.output.rootfs
					path:  "/dist"
				}
				output: _subdir.output
			}
			build: docker.#Build & {
				steps: [
					docker.#Pull & {
						source: "rancher/opni-python-base:3.8"
					},
					docker.#Copy & {
						contents: client.filesystem.".".read.contents
						source:   "aiops/"
						dest:     "."
					},
					docker.#Set & {
						config: {
							cmd: ["python", "opensearch-update-service/main.py"]
						}
					},
				]
			}
		}
	}
}
