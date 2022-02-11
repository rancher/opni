FROM alpine:3.15
COPY bin/opnim /usr/bin/opnim
COPY bin/plugin_* /var/lib/opnim/plugins
ENTRYPOINT ["/usr/bin/opnim"] 