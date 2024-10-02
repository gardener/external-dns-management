# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

#############      builder       #############
FROM golang:1.23.2 AS builder

WORKDIR /build

# Copy go mod and sum files
COPY go.mod go.sum ./
# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

COPY . .

RUN make release

############# base
FROM gcr.io/distroless/static-debian11:nonroot AS base

#############      dns-controller-manager     #############
FROM base AS dns-controller-manager
WORKDIR /

COPY --from=builder /build/dns-controller-manager /dns-controller-manager

WORKDIR /

USER 65534:65534

ENTRYPOINT ["/dns-controller-manager"]
