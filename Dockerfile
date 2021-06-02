FROM registry.opensuse.org/devel/bci/images/bci/golang:1.16 as builder

WORKDIR /workspace
COPY . .
RUN go mod download
RUN make

FROM registry.opensuse.org/devel/bci/images/bci/minimal:latest

WORKDIR /
COPY --from=builder /workspace/bin/manager .

ENTRYPOINT ["/manager"]