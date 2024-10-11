FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.5.0 AS xx

FROM --platform=$BUILDPLATFORM golang:1.23-alpine3.20 AS builder

COPY --from=xx / /

ARG TARGETPLATFORM

RUN xx-info env

ENV CGO_ENABLED=0

ENV XX_VERIFY_STATIC=1

WORKDIR /app

COPY . .

RUN xx-go build && \
    xx-verify gost-plugins

FROM alpine:3.20

WORKDIR /bin/

COPY --from=builder /app/gost-plugins .

ENTRYPOINT ["/bin/gost-plugins"]