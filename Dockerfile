FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

# Convert TARGETPLATFORM to GOARCH format
# https://github.com/tonistiigi/xx
COPY --from=tonistiigi/xx:golang / /

ARG TARGETPLATFORM

RUN apk add --no-cache musl-dev git gcc

ADD . /src

WORKDIR /src

ENV GO111MODULE=on

RUN go env && go build

FROM alpine:latest

WORKDIR /bin/

COPY --from=builder /src/gost-plugins .

ENTRYPOINT ["/bin/gost-plugins"]