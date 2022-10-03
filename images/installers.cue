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
					"apt update && apt -y install unzip curl && curl https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip -o aws.zip && unzip -qq aws.zip && ./aws/install && rm -rf aws",
				]
			}
		}
		#Helm: docker.#Run & {
			command: {
				name: "sh"
				args: [
					"-c",
					"apt update && apt -y install curl && curl -sfL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash",
				]
			}
		}
	}
}
