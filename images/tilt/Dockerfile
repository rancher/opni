FROM golang:1.17

ENV DOCKER_VERSION=20.10.5
ENV TILT_VERSION=0.22.9

RUN curl -sL https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz -o docker-${DOCKER_VERSION}.tgz \
  && tar -zxvf docker-${DOCKER_VERSION}.tgz -C /usr/bin docker/docker --strip-components 1 \
  && curl -fsSL https://github.com/tilt-dev/tilt/releases/download/v${TILT_VERSION}/tilt.${TILT_VERSION}.linux.x86_64.tar.gz | tar -xzv tilt \
  && mv tilt /usr/local/bin/tilt