package helm

import (
	"github.com/rancher/opni/images"
	"dagger.io/dagger"
	// "dagger.io/dagger/core"
	"universe.dagger.io/docker"
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
