# syntax=docker/dockerfile:1.29@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

#############      builder       #############
FROM --platform=$BUILDPLATFORM golang:1.27.4@sha256:5e8afd493a7124a8586e134d9b532731161d4b9becf9dee5a5956b107426f93d AS builder
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
FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931 AS base
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
