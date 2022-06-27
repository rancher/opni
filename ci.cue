@if(!test)
package main

import (
	"strings"
	"encoding/json"
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"universe.dagger.io/docker/cli"
	"universe.dagger.io/alpine"
	"universe.dagger.io/python"
	"universe.dagger.io/x/david@rawkode.dev/pulumi"
	"github.com/rancher/opni/internal/builders"
	"github.com/rancher/opni/internal/mage"
	"github.com/rancher/opni/internal/util"
	"github.com/rancher/opni/images"
)

dagger.#Plan & {
	client: {
		env: {
			GINKGO_LABEL_FILTER:    string | *""
			DRONE:                  string | *""
			KUBECONFIG:             string | *""
			TAG:                    string | *"latest"
			REPO:                   string | *"rancher"
			IMAGE_NAME:             string | *"opni"
			OPNI_UI_REPO:           string | *"rancher/opni-ui"
			OPNI_UI_BRANCH:         string | *"main"
			OPNI_UI_BUILD_IMAGE:    string | *"rancher/opni-monitoring-ui-build"
			DASHBOARDS_VERSION:     string | *"1.3.1"
			OPENSEARCH_VERSION:     string | *"1.3.1"
			PLUGIN_VERSION:         string | *"0.5.4-rc4"
			PLUGIN_PUBLISH:         string | *"0.5.4-rc4"
			EXPECTED_REF?:          string // used by tilt
			DOCKER_USERNAME?:       string
			DOCKER_PASSWORD?:       dagger.#Secret
			PULUMI_ACCESS_TOKEN?:   dagger.#Secret
			AWS_ACCESS_KEY_ID?:     dagger.#Secret
			AWS_SECRET_ACCESS_KEY?: dagger.#Secret
			TWINE_USERNAME:         string | *"__token__"
			TWINE_PASSWORD?:        dagger.#Secret
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
					"internal/cmd/testenv",
				]
			}
			"bin": write: contents:             actions.build.bin
			"web/dist": write: contents:        actions.web.dist
			"cover.out": write: contents:       actions.test.export.files["/src/cover.out"]
			"aiops/apis/dist": write: contents: actions.aiops.sdist.output
		}
		commands: {
			"aws-identity": {
				name: "aws"
				args: ["sts", "get-caller-identity"]
				stdout: string
			}
			"ecr-password": {
				name: "aws"
				args: ["ecr", "get-login-password", "--region", "us-east-2"]
				stdout: dagger.#Secret
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
					docker.#Run & {
						workdir: "/src"
						command: {
							name: "go"
							args: ["mod", "download"]
						}
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
				image: actions.aiops.opensearchUpdateService.output
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
		e2e: {
			_accessToken: client.env.PULUMI_ACCESS_TOKEN | *_|_
			_awsIdentity: json.Unmarshal(client.commands."aws-identity".stdout)

			_awsEnv: {
				"AWS_ACCESS_KEY_ID":     client.env.AWS_ACCESS_KEY_ID
				"AWS_SECRET_ACCESS_KEY": client.env.AWS_SECRET_ACCESS_KEY
			}
			_testImage: docker.#Push & {
				dest:  "\(_awsIdentity.Account).dkr.ecr.us-east-2.amazonaws.com/opni-e2e-test"
				image: _opniImage.image
				auth: {
					username: "AWS"
					secret:   client.commands."ecr-password".stdout
				}
			}

			_e2eImage: docker.#Build & {
				steps: [
					mage.#Run & {
						input: actions.build.output
						mageArgs: ["charts:charts", "opni"]
					},
					mage.#Run & {
						mageArgs: ["charts:charts", "opni-agent"]
					},
					images.installers.debian.#AwsCli,
				]
			}

			_fs: core.#Subdir & {
				input: _e2eImage.output.rootfs
				path:  "/src"
			}

			_pulumiImage: images.#Pulumi & {
				plugins: [
					images.#Plugin & {
						name:    "aws"
						version: "v5.9.1"
					},
					images.#Plugin & {
						name:    "aws"
						version: "v5.4.0"
					},
					images.#Plugin & {
						name:    "awsx"
						version: "v1.0.0-beta.8"
					},
					images.#Plugin & {
						name:    "kubernetes"
						version: "3.19.4"
					},
					images.#Plugin & {
						name:    "random"
						version: "v4.7.0"
					},
					images.#Plugin & {
						name:    "eks"
						version: "0.40.0"
					},
					images.#Plugin & {
						name:    "docker"
						version: "v3.2.0"
					},
				]
			}

			infra: pulumi.#Up & {
				source:      _fs.output
				version:     "3.34.1-debian-amd64"
				runtime:     "go"
				stack:       "e2e"
				accessToken: _accessToken
				container: {
					input: _pulumiImage.output
					env:   _awsEnv & {
						"CLOUD":      "aws"
						"IMAGE_REPO": _testImage.dest
						"IMAGE_TAG":  strings.TrimPrefix(_testImage.result, "\(_testImage.dest):")
					}
				}
			}

			test: mage.#Run & {
				input: _e2eImage.output
				mageArgs: ["-v", "test", "e2e"]
				env: _awsEnv & {
					"STACK_OUTPUTS": json.Marshal(infra.outputs)
				}
			}
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
			aiops: docker.#Push & {
				dest:  "\(client.env.REPO)/opni-opensearch-update-service:\(client.env.TAG)"
				image: actions.aiops.opensearchUpdateService.output
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
								"https://github.com/tybalex/opni-preprocessing-plugin/releases/download/v\(client.env.PLUGIN_VERSION)/opnipreprocessing.zip",
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
			sdist: {
				_dist: python.#Run & {
					script: {
						directory: actions.build.output.rootfs
						filename:  "src/aiops/apis/setup.py"
					}
					workdir: "/run/python/src/aiops/apis"
					args: ["sdist", "-d", "/dist"]
					output: docker.#Image
				}
				_distSubdir: core.#Subdir & {
					input: _dist.output.rootfs
					path:  "/dist"
				}
				image:  _dist.output
				output: dagger.#FS & _distSubdir.output
			}
			_distImage: docker.#Build & {
				steps: [
					docker.#Run & {
						input:  sdist.image
						command: {
							name: "pip"
							args: [ "install", "twine"]
						}
					},
				]
			}
			upload: docker.#Run & {
				input: _distImage.output
				command: {
					name: "twine"
					args: [
						"upload",
						"--repository",
						"pypi",
						"/dist/*",
					]
				}
				env: {
					TWINE_USERNAME: client.env.TWINE_USERNAME
					TWINE_PASSWORD: client.env.TWINE_PASSWORD
				}
			}
			opensearchUpdateService: docker.#Build & {
				steps: [
					docker.#Pull & {
						source: "rancher/opni-python-base:3.8"
					},
					docker.#Copy & {
						contents: client.filesystem.".".read.contents
						source:   "aiops/"
						dest:     "."
					},
					docker.#Run & {
						command: {
							name: "pip"
							args: ["install", "-r", "requirements.txt"]
						}
						env: HACK: "\(upload.success)"
					},
					docker.#Set & {
						config: {
							cmd: ["python", "opni-opensearch-update-service/opensearch-update-service/app/main.py"]
						}
					},
				]
			}
		}
	}
}
