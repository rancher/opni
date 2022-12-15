package helm

import (
	"github.com/rancher/opni/images"
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"universe.dagger.io/alpine"
	"github.com/rancher/opni/internal/mage"
)

// Upload an image to a remote repository
#Push: {
	// Source fs containing the chart
	source: dagger.#FS

	// Path within the source fs to the chart
	chart: string

	// Remote repository to upload to
	remote: string

	// Registry authentication
	auth?: {
		username: string
		secret:   dagger.#Secret
	}

	_exec: docker.#Build & {
		steps: [
			images.#Helm,
			docker.#Copy & {
				contents: source
				dest:     "/src"
			},
			docker.#Run & {
				always: true
				command: {
					name: "sh"
					args: [
						"-c",
						"/usr/local/bin/helm registry login -u ${USERNAME} -p ${PASSWORD} \(remote) && /usr/local/bin/helm push \(chart) oci://\(remote)",
					]
				}
				workdir: "/src"
				if auth != _|_ {
					env: {
						"USERNAME": auth.username
						"PASSWORD": auth.secret
					}
				}
			},
		]
	}
	output: _exec.output
}

#PublishToChartsRepo: {
	dev:    bool
	source: dagger.#FS
	remote: string | *"rancher"
	branch: string
	{
		dev:    true
		branch: "charts-repo-dev"
	} | {
		dev:    false
		branch: "charts-repo"
	}
	auth?: {
		user:  string
		email: string
		token: dagger.#Secret
	}

	_git: alpine.#Build & {
		version: "edge"
		packages: "github-cli": _
	}

	_repo: docker.#Run & {
		always: true
		input:  _git.output
		env: {
			"GH_TOKEN": auth.token
		}
		workdir: "/"
		command: {
			name: "sh"
			args: [
				"-c",
				"gh repo clone \(remote)/opni -- --branch \(branch)",
			]
		}
		export: directories: "/opni": _
	}

	_exec: docker.#Build & {
		steps: [
			mage.#Image,
			docker.#Copy & {
				contents: source
				"source": "/src"
				dest:     "/src"
			},
			docker.#Copy & {
				contents: _repo.export.directories["/opni"]
				dest:     "/src"
				include: ["charts/", "assets/", "index.yaml"]
			},
			docker.#Run & {
				workdir: "/src"
				command: {
					name: "mage"
					args: [
						"charts:index",
					]
				}
			},
		]
	}

	_sync: core.#Copy & {
		input:    _repo.export.directories["/opni"]
		contents: _exec.output.rootfs
		source:   "/src"
		include: ["charts/", "assets/", "index.yaml"]
	}

	_push: docker.#Run & {
		always: true
		input:  _git.output
		mounts: "charts-repo": {
			contents: _sync.output
			dest:     "/src"
		}
		env: {
			"GH_TOKEN":         auth.token
			"GIT_AUTHOR_NAME":  auth.user
			"GIT_AUTHOR_EMAIL": auth.email
		}
		workdir: "/src"
		command: {
			name: "sh"
			args: [
				"-c",
				"""
				gh auth setup-git && \\
				git config user.name \"$GIT_AUTHOR_NAME\" && \\
				git config user.email \"$GIT_AUTHOR_EMAIL\" && \\
				git add charts/ assets/ index.yaml && \\
				git diff --cached --quiet || (git commit -m 'Update charts' && git push origin refs/heads/\(branch):refs/heads/\(branch))
				""",
			]
		}
	}
}
