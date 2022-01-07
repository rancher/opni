FROM golang:1.17 as builder

WORKDIR /workspace
RUN go install github.com/magefile/mage@latest
COPY . .
RUN mage
RUN mv bin/opnim /

FROM alpine 
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /opnim /
ENTRYPOINT ["/opnim"]