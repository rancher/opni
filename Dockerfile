FROM alpine:3.15
COPY bin/opnim /usr/bin/opnim
ENTRYPOINT ["/usr/bin/opnim"] 