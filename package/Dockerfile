FROM registry.opensuse.org/devel/bci/images/bci/minimal:latest

WORKDIR /
COPY manager .
COPY ./assets/gpu-operator /opt/gpu-operator/

ENTRYPOINT ["/manager"]