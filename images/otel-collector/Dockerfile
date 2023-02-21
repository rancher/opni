FROM golang:1.20 AS builder

RUN go install go.opentelemetry.io/collector/cmd/builder@v0.71.0

WORKDIR /otel

COPY . .

RUN builder --config=ocb-config.yaml

FROM alpine:latest as prep
RUN apk --update add ca-certificates

RUN mkdir -p /tmp

FROM scratch

ARG USER_UID=10001
USER ${USER_UID}

COPY --from=prep /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /otel/otelcol-custom /

ENTRYPOINT [ "otelcol-custom" ]
CMD ["--config", "/etc/otel/config.yaml"]