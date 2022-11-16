@if(!test)
package main

import (
	"strings"
	"encoding/json"
	"encoding/yaml"
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"universe.dagger.io/docker/cli"
	"universe.dagger.io/alpine"
	"universe.dagger.io/python"
	"universe.dagger.io/alpha/pulumi"
	"github.com/rancher/opni/internal/builders"
	"github.com/rancher/opni/internal/mage"
	"github.com/rancher/opni/internal/util"
	"github.com/rancher/opni/internal/helm"
	"github.com/rancher/opni/images"
)

dagger.#Plan & {
	client: {
		env: {
			GINKGO_LABEL_FILTER:    string | *""
			DRONE:                  string | *""
			KUBECONFIG:             string | *""
			BUILD_VERSION:          string | *"unversioned"
			TAG:                    string | *"latest"
			REPO:                   string | *"rancher"
			IMAGE_NAME:             string | *"opni"
			OPNI_UI_REPO:           string | *"rancher/opni-ui"
			OPNI_UI_BRANCH:         string | *"main"
			OPNI_UI_BUILD_IMAGE:    string | *"rancher/opni-monitoring-ui-build"
			HELM_OCI_REPO:          string | *"ghcr.io/rancher"
			DASHBOARDS_VERSION:     string | *"1.3.3"
			OPENSEARCH_VERSION:     string | *"1.3.3"
			PLUGIN_VERSION:         string | *"0.6.2"
			PLUGIN_PUBLISH:         string | *"0.6.2"
			EXPECTED_REF?:          string // used by tilt
			DOCKER_USERNAME?:       string
			DOCKER_PASSWORD?:       dagger.#Secret
			HELM_OCI_USERNAME?:     string
			HELM_OCI_PASSWORD?:     dagger.#Secret
			PULUMI_ACCESS_TOKEN?:   dagger.#Secret
			AWS_ACCESS_KEY_ID?:     dagger.#Secret
			AWS_SECRET_ACCESS_KEY?: dagger.#Secret
			TWINE_USERNAME:         string | *"__token__"
			TWINE_PASSWORD?:        dagger.#Secret
			WEB_DEBUG:              string | *"false"
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
			"bin": write: contents:             actions.build.export.directories."/src/bin"
			"web/dist": write: contents:        actions.web.dist
			"cover.out": write: contents:       actions.test.export.files["/src/cover.out"]
			"aiops/apis/dist": write: contents: actions.pypi.sdist.output
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
			keepSourceMaps: client.env.WEB_DEBUG == "true"
		}

		_mageImage: mage.#Image

		// Build with mage using the builder image

		#_build: {
			target:   string | *"all"
			_minimal: target == "minimal"
			_exec:    docker.#Build & {
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
				]
			}
			docker.#Run & {
				input:   _exec.output
				workdir: "/src"
				env: {
					"BUILD_VERSION": client.env.BUILD_VERSION
				}
				command: {
					name: "mage"
					args: ["-v", target]
				}
				export: directories: {
					"/opt":             _
					"/src/bin":         _
					"/src/bin/plugins": _
					if target == "charts" {
						"/src/charts": _
						"/src/assets": _
					}
				}
			}
		}

		#_defaultBuild: #_build & {
			target: "all"
		}
		#_minimalBuild: #_build & {
			target: "minimal"
		}
		#_chartsBuild: #_build & {
			target: "charts"
		}

		_defaultBuild: #_defaultBuild
		_minimalBuild: #_minimalBuild
		_chartsBuild:  #_chartsBuild

		build:        _defaultBuild
		minimalBuild: _minimalBuild
		chartsBuild:  _chartsBuild

		// Build the destination base image
		_baseimage: alpine.#Build & {
			packages: {
				"bash":            _
				"ca-certificates": _
				"curl":            _
				"tini":            _
			}
		}

		// Copy the build output to the destination image
		#_multistage: {
			sourceBuild: #_build
			docker.#Build & {
				steps: [
					docker.#Copy & {
						input:    _baseimage.output
						contents: sourceBuild.export.directories."/src/bin"
						source:   "opni"
						dest:     "/usr/bin/opni"
					},
					docker.#Copy & {
						// input connects to previous step's output
						contents: sourceBuild.export.directories."/src/bin/plugins"
						dest:     "/var/lib/opni/plugins/"
						exclude: ["plugin_example"]
					},
					docker.#Copy & {
						contents: sourceBuild.export.directories."/opt"
						dest:     "/opt/"
					},
					docker.#Run & {
						command: {
							name: "sh"
							args: ["-c",
								"curl -sfL -o /etc/profile.d/bash_completion.sh https://raw.githubusercontent.com/scop/bash-completion/master/bash_completion && " +
								"/usr/bin/opni completion bash > /etc/profile.d/opni_bash_completion.sh",
							]
						}
					},
					docker.#Set & {
						config: {
							entrypoint: ["/sbin/tini", "--", "/usr/bin/opni"]
							env: {
								NVIDIA_VISIBLE_DEVICES: "void"
							}
						}
					},
				]
			}
		}

		_opniImage: {
			tag: docker.#Ref
			if client.env.EXPECTED_REF == _|_ {
				tag: docker.#Ref & "\(client.env.REPO)/\(client.env.IMAGE_NAME):\(client.env.TAG)"
			}
			if client.env.EXPECTED_REF != _|_ {
				tag: docker.#Ref & client.env.EXPECTED_REF
			}
			_out: #_multistage & {
				sourceBuild: _defaultBuild
			}
			image: _out.output
		}

		_minimalOpniImage: {
			tag: docker.#Ref
			if client.env.EXPECTED_REF == _|_ {
				tag: docker.#Ref & "\(client.env.REPO)/\(client.env.IMAGE_NAME):\(client.env.TAG)-minimal"
			}
			if client.env.EXPECTED_REF != _|_ {
				tag: docker.#Ref & client.env.EXPECTED_REF
			}
			_out: #_multistage & {
				sourceBuild: _minimalBuild
			}
			image: _out.output
		}

		// Build docker images and load them into the local docker daemon
		load: {
			opni: cli.#Load & _opniImage & {
				host: client.network."unix:///var/run/docker.sock".connect
			}
			opniMinimal: cli.#Load & _minimalOpniImage & {
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
			testImages: {
				repo: "\(_awsIdentity.Account).dkr.ecr.us-east-2.amazonaws.com/opni-e2e-test"
				_auth: {
					username: "AWS"
					secret:   client.commands."ecr-password".stdout
				}
				default: docker.#Push & {
					dest:  repo + ":latest"
					image: _opniImage.image
					auth:  _auth
				}
				minimal: docker.#Push & {
					dest:  repo + ":latest-minimal"
					image: _minimalOpniImage.image
					auth:  _auth
				}
			}

			_e2eImage: docker.#Build & {
				steps: [
					images.installers.debian.#AwsCli & {
						_input:  _chartsBuild
						input:   _input.output
						workdir: "/src"
					},
				]
			}

			_fs: core.#Subdir & {
				input: _e2eImage.output.rootfs
				path:  "/src"
			}

			_pulumiImage: images.#Pulumi & {
				plugins: [
					images.#Plugin & {
						name: "aws"
					},
					images.#Plugin & {
						name:    "aws"
						version: "v5.4.0"
					},
					images.#Plugin & {
						name:    "awsx"
						version: "v1.0.0-beta.9"
					},
					images.#Plugin & {
						name: "kubernetes"
					},
					images.#Plugin & {
						name: "random"
					},
					images.#Plugin & {
						name: "eks"
					},
					images.#Plugin & {
						name: "docker"
					},
				]
			}

			infra: {
				_defaultImageTag: testImages.default.result
				_minimalImageTag: testImages.minimal.result
				_up:              pulumi.#Up & {
					source:      _fs.output
					runtime:     "go"
					stack:       "e2e"
					accessToken: _accessToken
					container: {
						input: _pulumiImage.output
						env:   _awsEnv & {
							"CLOUD":             "aws"
							"IMAGE_REPO":        testImages.repo
							"IMAGE_TAG":         strings.TrimPrefix(_defaultImageTag, "\(testImages.repo):")
							"MINIMAL_IMAGE_TAG": strings.TrimPrefix(_minimalImageTag, "\(testImages.repo):")
						}
					}
				}
				_stackOutput: core.#Subdir & {
					input: _up.container.output.rootfs
					path:  "/output"
				}
				stackOutputSecret: core.#NewSecret & {
					input: _stackOutput.output
					path:  "json"
				}
			}

			test: mage.#Run & {
				input: _e2eImage.output
				mageArgs: ["-v", "test", "e2e"]
				env: _awsEnv & {
					"STACK_OUTPUTS": infra.stackOutputSecret.output
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
			opniMinimal: docker.#Push & {
				dest:  _minimalOpniImage.tag
				image: _minimalOpniImage.image
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

			_agentPackageConfig: core.#ReadFile & {
				input: client.filesystem.".".read.contents
				path:  "packages/opni-agent/opni-agent/package.yaml"
			}
			_agentChartVersion: yaml.Unmarshal(_agentPackageConfig.contents).version

			charts: {
				_auth?: {
					username: string
					secret:   dagger.#Secret
				}
				if client.env.HELM_OCI_USERNAME != _|_ && client.env.HELM_OCI_PASSWORD != _|_ {
					_auth: {
						username: client.env.HELM_OCI_USERNAME
						secret:   client.env.HELM_OCI_PASSWORD
					}
				}
				if client.env.HELM_OCI_USERNAME == _|_ && client.env.HELM_OCI_PASSWORD == _|_ &&
					client.env.DOCKER_USERNAME != _|_ && client.env.DOCKER_PASSWORD != _|_ {
					_auth: {
						username: client.env.DOCKER_USERNAME
						secret:   client.env.DOCKER_PASSWORD
					}
				}

				agent: helm.#Push & {
					source: _chartsBuild.export.directories."/src/assets"
					chart:  "opni-agent/opni-agent-\(_agentChartVersion).tgz"
					remote: client.env.HELM_OCI_REPO
					if _auth != _|_ {
						auth: _auth
					}
				}
				agentCrd: helm.#Push & {
					source: _chartsBuild.export.directories."/src/assets"
					chart:  "opni-agent-crd/opni-agent-crd-\(_agentChartVersion).tgz"
					remote: client.env.HELM_OCI_REPO
					if _auth != _|_ {
						auth: _auth
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
					docker.#Run & {
						command: {
							name: "opensearch-plugin"
							args: [
								"-s",
								"install",
								"-b",
								"https://github.com/tybalex/opni-preprocessing-plugin/releases/download/v\(client.env.PLUGIN_VERSION)/opnijsondetector.zip",
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
					},
					docker.#Set & {
						config: {
							cmd: ["python", "opni-opensearch-update-service/opensearch-update-service/app/main.py"]
						}
					},
				]
			}
			push: docker.#Push & {
				dest:  "\(client.env.REPO)/opni-opensearch-update-service:\(client.env.TAG)"
				image: opensearchUpdateService.output
				if client.env.DOCKER_USERNAME != _|_ && client.env.DOCKER_PASSWORD != _|_ {
					auth: {
						username: client.env.DOCKER_USERNAME
						secret:   client.env.DOCKER_PASSWORD
					}
				}
			}
		}
		pypi: {
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
						input: sdist.image
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
		}
	}
}
