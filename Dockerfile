FROM alpine

COPY bin/opni-gateway /

ENTRYPOINT ["/opni-gateway"]