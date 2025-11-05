# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

ENSURE_GARDENER_MOD               := $(shell go get github.com/gardener/gardener@$$(go list -m -f "{{.Version}}" github.com/gardener/gardener))
GARDENER_HACK_DIR                 := $(shell go list -m -f "{{.Dir}}" github.com/gardener/gardener)/hack
ENSURE_CONTROLLER_MANAGER_LIB_MOD := $(shell go get github.com/gardener/controller-manager-library@$$(go list -m -f "{{.Version}}" github.com/gardener/controller-manager-library))
CONTROLLER_MANAGER_LIB_HACK_DIR   := $(shell go list -m -f "{{.Dir}}" github.com/gardener/controller-manager-library)/hack
REGISTRY                          := europe-docker.pkg.dev/gardener-project/public
EXECUTABLE                        := dns-controller-manager
EXECUTABLE2                       := dns-controller-manager-next-generation
REPO_ROOT                         := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
HACK_DIR                          := $(REPO_ROOT)/hack
PROJECT                           := github.com/gardener/external-dns-management
IMAGE_REPOSITORY                  := $(REGISTRY)/dns-controller-manager
IMAGE_REPOSITORY_NG               := $(REGISTRY)/dns-controller-manager-next-generation
VERSION                           := $(shell cat VERSION)
IMAGE_TAG                         := $(VERSION)

#########################################
# Tools                                 #
#########################################

TOOLS_DIR := hack/tools
include $(GARDENER_HACK_DIR)/tools.mk

.PHONY: tidy
tidy:
	@go mod tidy
	@cp $(GARDENER_HACK_DIR)/sast.sh $(HACK_DIR)/sast.sh && chmod +xw $(HACK_DIR)/sast.sh

.PHONY: clean
clean:
	bash $(GARDENER_HACK_DIR)/clean.sh ./cmd/... ./pkg/...
	@rm -f charts/external-dns-management/templates/crds.yaml
	@rm -f pkg/apis/dns/crds/*
	@rm -rf /pkg/client/dns

.PHONY: check-generate
check-generate:
	@bash $(GARDENER_HACK_DIR)/check-generate.sh $(REPO_ROOT)

.PHONY: check-imports
check-imports: $(IMPORT_BOSS)
	$(IMPORT_BOSS) ./cmd/... ./pkg/... ./test/...

.PHONY: check
check: format check-imports $(GOIMPORTS) $(GOLANGCI_LINT)
	@TOOLS_BIN_DIR="$(TOOLS_BIN_DIR)" bash $(CONTROLLER_MANAGER_LIB_HACK_DIR)/check.sh --golangci-lint-config=./.golangci.yaml ./cmd/... ./pkg/... ./test/...
	@echo "Running go vet..."
	@go vet ./cmd/... ./pkg/... ./test/...

.PHONY: format
format: $(GOIMPORTS) $(GOIMPORTSREVISER)
	@bash $(GARDENER_HACK_DIR)/format.sh ./cmd ./pkg ./test

.PHONY: build-local
build-local:
	@CGO_ENABLED=1 go build -o $(EXECUTABLE) \
	    -race \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/compound
	@CGO_ENABLED=1 go build -o $(EXECUTABLE2) \
	    -race \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/nextgen

.PHONY: release
release:
	@CGO_ENABLED=0 go build -o $(EXECUTABLE) \
	    -a \
	    -ldflags "-w -X main.Version=$(VERSION)" \
	    ./cmd/compound
	@CGO_ENABLED=0 go build -o $(EXECUTABLE2) \
	    -a \
	    -ldflags "-w -X main.Version=$(VERSION)" \
	    ./cmd/nextgen

.PHONY: unittests
unittests: $(GINKGO)
	hack/go-test.sh -race -timeout=3m ./pkg/...

.PHONY: new-test-integration
new-test-integration: $(REPORT_COLLECTOR) $(SETUP_ENVTEST)
	@bash $(GARDENER_HACK_DIR)/test-integration.sh ./test/integration/compound ./test/integration/source ./test/integration/dnsman2

.PHONY: test
test: $(GINKGO) unittests new-test-integration
	@echo ----- Skipping long running integration tests, use \'make alltests\' to run all tests -----
	GINKGO=$(shell realpath $(GINKGO)) test/integration/run.sh -l $(kindargs) -- -skip Many $(args)

.PHONY: generate-proto
generate-proto:
	@protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    --experimental_allow_proto3_optional \
    pkg/server/remote/common/remote.proto

.PHONY: generate
generate: $(VGOPATH) $(CONTROLLER_GEN) $(HELM) $(YQ)
	@CONTROLLER_MANAGER_LIB_HACK_DIR=$(CONTROLLER_MANAGER_LIB_HACK_DIR) VGOPATH=$(VGOPATH) REPO_ROOT=$(REPO_ROOT) ./hack/generate-code
	@CONTROLLER_MANAGER_LIB_HACK_DIR=$(CONTROLLER_MANAGER_LIB_HACK_DIR) CONTROLLER_GEN=$(shell realpath $(CONTROLLER_GEN)) go generate ./pkg/apis/dns/...
	@CONTROLLER_MANAGER_LIB_HACK_DIR=$(CONTROLLER_MANAGER_LIB_HACK_DIR) CONTROLLER_GEN=$(shell realpath $(CONTROLLER_GEN))  HELM=$(shell realpath $(HELM)) go generate ./charts/external-dns-management
	@./hack/copy-crds.sh
	@GARDENER_HACK_DIR=$(GARDENER_HACK_DIR) VGOPATH=$(VGOPATH) REPO_ROOT=$(REPO_ROOT) CONTROLLER_GEN=$(shell realpath $(CONTROLLER_GEN)) go generate ./pkg/dnsman2/apis/...
	$(MAKE) format
	$(MAKE) tidy
	@./hack/generate-renovate-ignore-deps.sh

.PHONY: alltests
alltests: $(GINKGO)
	$(GINKGO) -r ./pkg
	GINKGO=$(shell realpath $(GINKGO)) test/integration/run.sh $(kindargs) -- $(args)

.PHONY: integrationtests
integrationtests: $(GINKGO)
	GINKGO=$(shell realpath $(GINKGO)) test/integration/run.sh -l $(kindargs) -- $(args)

.PHONY: test-integration
test-integration: integrationtests new-test-integration

.PHONY: docker-images
docker-images:
	@docker build -t $(IMAGE_REPOSITORY):$(IMAGE_TAG) -f Dockerfile --target dns-controller-manager .
	@docker build -t $(IMAGE_REPOSITORY_NG):$(IMAGE_TAG) -f Dockerfile --target dns-controller-manager-next-generation .

.PHONY: sast
sast: $(GOSEC)
	@./hack/sast.sh --exclude-dirs hack,local

.PHONY: sast-report
sast-report: $(GOSEC)
	@./hack/sast.sh --exclude-dirs hack,local --gosec-report true

.PHONY: verify
verify: check format sast

.PHONY: verify-extended
verify-extended: check-generate check format sast-report
