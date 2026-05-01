# Multi-stage Dockerfile for monorel.
#
# The image is published to ghcr.io/disaresta-org/monorel on every
# release tag. Use it as an alternative to the platform binaries on
# macOS / Windows where the unsigned binaries trigger Gatekeeper /
# SmartScreen warnings — the container ships the same monorel binary
# but runs in Linux, sidestepping the OS-level signing checks.
#
# Usage from the host repo root:
#   docker run --rm -v "$PWD:/workspace" \
#     -e GITHUB_TOKEN \
#     ghcr.io/disaresta-org/monorel:latest plan

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X monorel.disaresta.com/internal/cli.Version=${VERSION}" \
    -o /out/monorel ./cmd/monorel

FROM alpine:3.20
# `monorel` shells out to git.
RUN apk add --no-cache git ca-certificates && \
    update-ca-certificates 2>/dev/null || true
COPY --from=build /out/monorel /usr/local/bin/monorel
WORKDIR /workspace
ENTRYPOINT ["monorel"]

LABEL org.opencontainers.image.source="https://github.com/disaresta-org/monorel"
LABEL org.opencontainers.image.description="A changesets-style release tool for multi-module Go monorepos."
LABEL org.opencontainers.image.licenses="MIT"
