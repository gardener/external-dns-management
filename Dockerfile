# syntax=docker/dockerfile:1.27@sha256:bde3983e9c939224420ddaf6b784cc30e09b035a4dea01f581230c50809f372e
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

#############      builder       #############
FROM --platform=$BUILDPLATFORM golang:1.26.7@sha256:45a5f7a810238aabcbad211d70b9ae082022d96f7c7259e94041ad1b933575ac AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

# Copy go mod and sum files
COPY go.mod go.sum ./
# Download all dependencies. Cached via BuildKit cache mount independent of layer cache.
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH make release

############# base
FROM gcr.io/distroless/static-debian13:nonroot AS base
WORKDIR /
USER nonroot:nonroot

#############      dns-controller-manager     #############
FROM base AS dns-controller-manager

COPY --from=builder /build/dns-controller-manager /dns-controller-manager

ENTRYPOINT ["/dns-controller-manager"]

#############      dns-controller-manager-next-generation     #############
FROM base AS dns-controller-manager-next-generation

COPY --from=builder /build/dns-controller-manager-next-generation /dns-controller-manager-next-generation

ENTRYPOINT ["/dns-controller-manager-next-generation"]
