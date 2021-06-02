FROM registry.opensuse.org/devel/bci/images/bci/golang:1.16 as builder

WORKDIR /workspace
COPY . .
RUN ls; go mod download
RUN make opni

FROM registry.opensuse.org/devel/bci/images/bci/minimal:latest

WORKDIR /
COPY --from=builder /workspace/bin/opni .
USER 65532:65532

ENTRYPOINT ["/opni"]