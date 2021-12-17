FROM golang:1.17 as builder

WORKDIR /workspace
COPY . .
RUN go install github.com/magefile/mage@latest && mage && mv bin/opni-gateway /

FROM alpine 

COPY --from=builder /opni-gateway /
ENTRYPOINT ["/opni-gateway"]