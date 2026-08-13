# syntax=docker/dockerfile:1.26@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

#############      builder       #############
FROM --platform=$BUILDPLATFORM golang:1.26.6@sha256:640a234f4bea3e399c056b7b8f9c667c4939befae8db2f14e9785e16eccd4205 AS builder
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
