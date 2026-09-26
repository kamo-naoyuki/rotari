FROM golang:1.26-alpine AS build

ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN mkdir -p /out \
    && CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/rotari ./cmd/rotari

FROM alpine:3.22

RUN apk add --no-cache bash ca-certificates openssh-client \
    && addgroup -S -g 10001 rotari \
    && adduser -S -D -H -u 10001 -G rotari -h /home/rotari rotari \
    && mkdir -p /home/rotari /var/lib/rotari /workspace \
    && chown -R rotari:rotari /home/rotari /var/lib/rotari /workspace

COPY --from=build /out/rotari /usr/local/bin/rotari

ENV HOME=/home/rotari \
    ROTARI_BASEDIR=/var/lib/rotari/state \
    ROTARI_MASTERDIR=/var/lib/rotari/master

WORKDIR /workspace
USER 10001:10001
ENTRYPOINT ["rotari"]
