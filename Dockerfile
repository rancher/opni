FROM registry.opensuse.org/devel/bci/images/bci/golang:1.16 as builder

WORKDIR /workspace
COPY . .
RUN go mod download
RUN scripts/manager

FROM nvidia/cuda:11.4.1-base-centos8

ENV NVIDIA_VISIBLE_DEVICES=void

WORKDIR /
COPY --from=builder /workspace/bin/manager .
COPY --from=builder /workspace/package/assets/nfd /opt/nfd/
COPY --from=builder /workspace/package/assets/gpu-operator /opt/gpu-operator/

ENTRYPOINT ["/manager"]
