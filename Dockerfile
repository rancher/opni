FROM golang:1.17 as builder

WORKDIR /workspace
RUN go install github.com/magefile/mage@latest
COPY . .
RUN mage
RUN mv bin/opni-gateway /

FROM alpine 
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /opni-gateway /
ENTRYPOINT ["/opni-gateway"]