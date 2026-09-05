# Build the SPA, then the Go binary, then a small runtime image.
#
# NOTE: on-access scanning and systemd service management need host systemd and
# fanotify, so the container needs --privileged and the host's systemd socket.
# The supported deployment is the bare-metal install (scripts/install.sh / .deb).

FROM node:20-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /internal/webui/dist ./internal/webui/dist
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/clamav-webui ./cmd/clamav-webui

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates clamav clamav-daemon clamav-freshclam \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/clamav-webui /usr/bin/clamav-webui
ENV CLAMWEB_CONFIG_DIR=/var/lib/clamav-webui CLAMWEB_ADDR=:8080
VOLUME /var/lib/clamav-webui
EXPOSE 8080
ENTRYPOINT ["/usr/bin/clamav-webui"]
