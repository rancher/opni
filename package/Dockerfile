FROM registry.suse.com/suse/sle15:15.3

WORKDIR /
COPY manager .
COPY ./assets/gpu-operator /opt/gpu-operator/

ENTRYPOINT ["/manager"]