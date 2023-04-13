FROM mcr.microsoft.com/vscode/devcontainers/go:1.20-bullseye

RUN git clone https://github.com/magefile/mage
RUN cd mage && go run bootstrap.go

RUN go install github.com/onsi/ginkgo/v2/ginkgo@latest

RUN cd /usr/local
RUN curl -L https://dl.dagger.io/dagger-cue/install.sh | sh

RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
RUN curl -LO "https://dl.k8s.io/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl.sha256"
RUN echo "$(cat kubectl.sha256)  kubectl" | sha256sum --check
RUN sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

RUN curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
RUN curl -fsSL https://raw.githubusercontent.com/tilt-dev/tilt/master/scripts/install.sh | bash
