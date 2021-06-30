REGISTRY              := eu.gcr.io/gardener-project
EXECUTABLE            := dns-controller-manager
PROJECT               := github.com/gardener/external-dns-management
IMAGE_REPOSITORY      := $(REGISTRY)/dns-controller-manager
VERSION               := $(shell cat VERSION)
IMAGE_TAG             := $(VERSION)
EFFECTIVE_VERSION     := $(VERSION)-$(shell git rev-parse HEAD)

.PHONY: revendor
revendor:
	@GO111MODULE=on go mod vendor
	@GO111MODULE=on go mod tidy


.PHONY: check
check:
	@.ci/check

.PHONY: build
build:
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -o $(EXECUTABLE) \
	    -mod=vendor \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/compound

.PHONY: build-local
build-local:
	@CGO_ENABLED=1 GO111MODULE=on go build -o $(EXECUTABLE) \
	    -mod=vendor \
	    -race \
	    -gcflags="all=-N -l" \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/compound

.PHONY: build-local-dedicated
build-local-dedicated:
	@CGO_ENABLED=1 GO111MODULE=on go build -o $(EXECUTABLE)-dedicated \
	    -mod=vendor \
	    -race \
	    -gcflags="all=-N -l" \
	    -ldflags "-X main.Version=$(VERSION)-$(shell git rev-parse HEAD)"\
	    ./cmd/dedicated

.PHONY: release
release:
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -o $(EXECUTABLE) \
	    -a \
	    -mod=vendor \
	    -ldflags "-w -X main.Version=$(VERSION)" \
	    ./cmd/compound

.PHONY: test
test:
	GO111MODULE=on go test -mod=vendor ./pkg/...
	@echo ----- Skipping long running integration tests, use \'make alltests\' to run all tests -----
	test/integration/run.sh $(kindargs) -- -skip Many $(args)

.PHONY: generate
generate:
	@./hack/generate-code
	@GO111MODULE=on go generate ./pkg/apis/dns/...
	@GO111MODULE=on go generate ./charts/external-dns-management

.PHONY: install-requirements
install-requirements:
	@go install -mod=vendor github.com/onsi/ginkgo/ginkgo
	@GO111MODULE=off go get golang.org/x/tools/cmd/goimports
	@./hack/install-requirements.sh

alltests:
	GO111MODULE=on go test -mod=vendor ./pkg/...
	test/integration/run.sh $(kindargs) -- $(args)

.PHONY: docker-images
docker-images:
	@docker build -t $(IMAGE_REPOSITORY):$(IMAGE_TAG) -f build/Dockerfile .

#####################################################################
# Rules for cnudie component descriptors dev setup #
#####################################################################

.PHONY: cnudie-docker-images
cnudie-docker-images:
	@echo "Building docker images for version $(EFFECTIVE_VERSION) for registry $(IMAGE_REPOSITORY)"
	@docker build -t $(IMAGE_REPOSITORY):$(EFFECTIVE_VERSION) -f build/Dockerfile .

.PHONY: cnudie-docker-push
cnudie-docker-push:
	@echo "Pushing docker images for version $(EFFECTIVE_VERSION) to registry $(IMAGE_REPOSITORY)"
	@if ! docker images $(IMAGE_REPOSITORY) | awk '{ print $$2 }' | grep -q -F $(EFFECTIVE_VERSION); then echo "$(IMAGE_REPOSITORY) version $(EFFECTIVE_VERSION) is not yet built. Please run 'make cnudie-docker-images'"; false; fi
	@docker push $(IMAGE_REPOSITORY):$(EFFECTIVE_VERSION)

.PHONY: cnudie-docker-all
cnudie-docker-all: cnudie-docker-images cnudie-docker-push

.PHONY: cnudie-cd-build-push
cnudie-cd-build-push:
	@EFFECTIVE_VERSION=$(EFFECTIVE_VERSION) ./hack/generate-cd.sh

.PHONY: cnudie-create-installation
cnudie-create-installation:
	@EFFECTIVE_VERSION=$(EFFECTIVE_VERSION) ./hack/create-installation.sh


