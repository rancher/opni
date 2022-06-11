package images

import (
	"universe.dagger.io/docker"
)

installers: {
	debian: {
		#AwsCli: docker.#Run & {
			command: {
				name: "sh"
				args: [
					"-c",
					"apt update && apt -y install unzip && curl https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip -o aws.zip && unzip aws.zip && ./aws/install && rm -rf aws",
				]
			}
		}
	}
}
