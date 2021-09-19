FROM golang:1.17 as builder

RUN mkdir /artifacts \
  && git clone --branch master https://github.com/kralicky/gpu-operator \
  && cd gpu-operator/validator \
  && CGO_ENABLED=0 go build -ldflags='-w -s' -o /artifacts/nvidia-validator . \
  && cp cuda-workload-validation.yaml plugin-workload-validation.yaml /artifacts/

FROM nvidia/cuda:11.4.1-devel-ubuntu20.04 as sample-builder
RUN apt update && apt install -y --no-install-recommends cuda-samples-11-4 \
  && cd /usr/local/cuda/samples/0_Simple/vectorAdd \
  && make build EXTRA_CCFLAGS=-static-libstdc++,-static-libgcc \
  && mv vectorAdd /tmp

FROM ubuntu:20.04

RUN mkdir -p /var/nvidia/manifests
COPY --from=sample-builder /tmp/vectorAdd /usr/bin/vectorAdd
COPY --from=builder /artifacts/nvidia-validator /usr/bin/nvidia-validator
COPY --from=builder /artifacts/cuda-workload-validation.yaml /var/nvidia/manifests
COPY --from=builder /artifacts/plugin-workload-validation.yaml /var/nvidia/manifests

ENV NVIDIA_DISABLE_REQUIRE="true"
ENV NVIDIA_VISIBLE_DEVICES="all"
ENV NVIDIA_DRIVER_CAPABILITIES="compute,utility"

ENTRYPOINT ["/bin/bash"]
