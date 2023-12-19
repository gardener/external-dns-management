# SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

ENSURE_CONTROLLER_MANAGER_LIB_MOD := $(shell go get github.com/gardener/controller-manager-library@$$(go list -m -f "{{.Version}}" github.com/gardener/controller-manager-library))
CONTROLLER_MANAGER_LIB_HACK_DIR   := $(shell go list -m -f "{{.Dir}}" github.com/gardener/controller-manager-library)/hack
REGISTRY                          := europe-docker.pkg.dev/gardener-project/public
EXECUTABLE                        := dns-controller-manager
REPO_ROOT                         := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
PROJECT                           := github.com/gardener/external-dns-management
IMAGE_REPOSITORY                  := $(REGISTRY)/dns-controller-manager
VERSION                           := $(shell cat VERSION)
IMAGE_TAG                         := $(VERSION)

#########################################
# Tools                                 #
#########################################

TOOLS_DIR := hack/tools
include $(CONTROLLER_MANAGER_LIB_HACK_DIR)/tools.mk

HELM                       := $(TOOLS_BIN_DIR)/helm
HELM_VERSION ?= v3.13.1

$(HELM): $(call tool_version_file,$(HELM),$(HELM_VERSION))
	@curl -sSfL https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | HELM_INSTALL_DIR=$(TOOLS_BIN_DIR) PATH=$(PATH):$(TOOLS_BIN_DIR) USE_SUDO=false bash -s -- --version $(HELM_VERSION)


.PHONY: tidy
tidy:
	@go mod tidy

.PHONY: clean
clean:
	@rm -f charts/external-dns-management/templates/crds.yaml
	@rm -f pkg/apis/dns/crds/*
	@rm -rf /pkg/client/dns
	@rm -f pkg/apis/dns/v1alpha1/zz_generated*

.PHONY: check
check: format $(GOIMPORTS) $(GOLANGCI_LINT)
	@TOOLS_BIN_DIR="$(TOOLS_DIR)/bin" bash $(CONTROLLER_MANAGER_LIB_HACK_DIR)/check.sh --golangci-lint-config=./.golangci.yaml ./cmd/... ./pkg/... ./test/...
	@echo "Running go vet..."
	@go vet ./cmd/... ./pkg/... ./test/...

.PHONY: format
format:
	@go fmt ./cmd/... ./pkg/... ./test/...

.PHONY: build
build:
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(EXECUTABLE) \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/compound

.PHONY: build-local
build-local:
	@CGO_ENABLED=1 go build -o $(EXECUTABLE) \
	    -race \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/compound

.PHONY: build-local-dedicated
build-local-dedicated:
	@CGO_ENABLED=1 go build -o $(EXECUTABLE)-dedicated \
	    -race \
	    -gcflags="all=-N -l" \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/dedicated

.PHONY: release
release:
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(EXECUTABLE) \
	    -a \
	    -ldflags "-w -X main.Version=$(VERSION)" \
	    ./cmd/compound

.PHONY: unittests
unittests: $(GINKGO)
	$(GINKGO) -r ./pkg

.PHONY: test
test: $(GINKGO) unittests
	@echo ----- Skipping long running integration tests, use \'make alltests\' to run all tests -----
	GINKGO=$(shell realpath $(GINKGO)) test/integration/run.sh -l $(kindargs) -- -skip Many $(args)

.PHONY: generate-proto
generate-proto:
	@protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    --experimental_allow_proto3_optional \
    pkg/server/remote/common/remote.proto

.PHONY: generate
generate: $(VGOPATH) $(CONTROLLER_GEN) $(HELM)
	@CONTROLLER_MANAGER_LIB_HACK_DIR=$(CONTROLLER_MANAGER_LIB_HACK_DIR) VGOPATH=$(VGOPATH) REPO_ROOT=$(REPO_ROOT) ./hack/generate-code
	@CONTROLLER_MANAGER_LIB_HACK_DIR=$(CONTROLLER_MANAGER_LIB_HACK_DIR) CONTROLLER_GEN=$(shell realpath $(CONTROLLER_GEN)) go generate ./pkg/apis/dns/...
	@CONTROLLER_MANAGER_LIB_HACK_DIR=$(CONTROLLER_MANAGER_LIB_HACK_DIR) CONTROLLER_GEN=$(shell realpath $(CONTROLLER_GEN))  HELM=$(shell realpath $(HELM)) go generate ./charts/external-dns-management
	@./hack/copy-crds.sh
	@go fmt ./pkg/...

alltests: $(GINKGO)
	$(GINKGO) -r ./pkg
	GINKGO=$(shell realpath $(GINKGO)) test/integration/run.sh $(kindargs) -- $(args)

integrationtests: $(GINKGO)
	GINKGO=$(shell realpath $(GINKGO)) test/integration/run.sh -l $(kindargs) -- $(args)

.PHONY: docker-images
docker-images:
	@docker build -t $(IMAGE_REPOSITORY):$(IMAGE_TAG) -f Dockerfile --target dns-controller-manager .
